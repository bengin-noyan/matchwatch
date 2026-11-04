import hmac
import logging
import time

from fastapi import FastAPI, Request
from fastapi.concurrency import run_in_threadpool
from fastapi.responses import JSONResponse

from .config import Settings, load
from .extract import extract
from .llm import LLM, LLMError, OpenAICompatEmbedder, OpenAICompatLLM
from .pdftext import MAX_BYTES, PdfError, extract_pages, tesseract_ocr

log = logging.getLogger("matchwatch_ai")
_PDF_STATUS = {"NOT_PDF": 400, "TOO_LARGE": 413, "ENCRYPTED_PDF": 422}
MAX_TEXTS, MAX_TEXT_CHARS = 256, 512
# Fixed messages: never echo exception or upstream text (KVKK).
_MSG = {
    "NOT_PDF": "body is not a PDF",
    "TOO_LARGE": "PDF is too large",
    "ENCRYPTED_PDF": "PDF is encrypted",
    "LLM_UNAVAILABLE": "LLM is unavailable",
    "LLM_BAD_OUTPUT": "LLM returned unusable output",
    "UNAUTHORIZED": "missing or invalid token",
    "BAD_REQUEST": "texts must be a list of 1-256 non-empty strings of at most 512 characters",
    "INTERNAL": "internal error",
}


def _err(status: int, code: str) -> JSONResponse:
    return JSONResponse({"code": code, "error": _MSG.get(code, code)}, status_code=status)


def create_app(settings: Settings, llm: LLM | None = None, ocr=tesseract_ocr, embedder=None) -> FastAPI:
    llm = llm or OpenAICompatLLM(settings.llm_base_url, settings.llm_model, settings.llm_timeout_s)
    embedder = embedder or OpenAICompatEmbedder(settings.llm_base_url, settings.embed_model)
    app = FastAPI()

    def authorized(request: Request) -> bool:
        auth = request.headers.get("authorization", "")
        return hmac.compare_digest(auth.encode(), f"Bearer {settings.token}".encode())

    @app.get("/healthz")
    def healthz():
        return {"status": "ok"}

    @app.post("/v1/extract")
    async def extract_endpoint(request: Request):
        if not authorized(request):
            return _err(401, "UNAUTHORIZED")
        cl = request.headers.get("content-length", "")
        if cl.isdigit() and int(cl) > MAX_BYTES:
            return _err(413, "TOO_LARGE")
        buf = bytearray()
        async for chunk in request.stream():
            buf += chunk
            if len(buf) > MAX_BYTES:
                return _err(413, "TOO_LARGE")
        return await run_in_threadpool(_run, bytes(buf))

    @app.post("/v1/embed")
    async def embed_endpoint(request: Request):
        if not authorized(request):
            return _err(401, "UNAUTHORIZED")
        try:
            texts = (await request.json())["texts"]
        except Exception:  # noqa: BLE001 - any malformed body is a 400
            return _err(400, "BAD_REQUEST")
        if not (isinstance(texts, list) and 0 < len(texts) <= MAX_TEXTS and all(
                isinstance(t, str) and t.strip() and len(t) <= MAX_TEXT_CHARS for t in texts)):
            return _err(400, "BAD_REQUEST")
        return await run_in_threadpool(_embed, texts)

    def _embed(texts: list[str]):
        t0 = time.monotonic()
        code = "ok"
        try:
            return {"model": embedder.model, "vectors": embedder.embed(texts)}
        except LLMError as e:
            code = e.code
            return _err(502, e.code)
        except Exception as e:  # noqa: BLE001 - no message/traceback: may contain item names
            code = f"INTERNAL:{type(e).__name__}"
            return _err(500, "INTERNAL")
        finally:
            log.info("embed n=%d ms=%d code=%s", len(texts), int((time.monotonic() - t0) * 1000), code)

    def _run(data: bytes):
        t0 = time.monotonic()
        pages = ocr_pages = 0
        code = "ok"
        try:
            out = extract(extract_pages(data, ocr=ocr), llm, settings.max_chars, settings.extract_deadline_s)
            pages, ocr_pages = out["meta"]["pages"], len(out["meta"]["ocr_pages"])
            out["meta"]["duration_ms"] = int((time.monotonic() - t0) * 1000)
            return out
        except PdfError as e:
            code = e.code
            return _err(_PDF_STATUS.get(e.code, 400), e.code)
        except LLMError as e:
            code = e.code
            return _err(502, e.code)
        except Exception as e:  # noqa: BLE001 - no message/traceback: may contain document text
            code = f"INTERNAL:{type(e).__name__}"
            return _err(500, "INTERNAL")
        finally:
            log.info("extract pages=%d bytes=%d ocr=%d ms=%d code=%s", pages, len(data), ocr_pages,
                     int((time.monotonic() - t0) * 1000), code)

    return app


def factory() -> FastAPI:
    logging.basicConfig(level=logging.INFO)  # summary line only, never content
    return create_app(load())
