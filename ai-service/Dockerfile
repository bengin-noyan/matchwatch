FROM python:3.12-slim
RUN apt-get update && apt-get install -y --no-install-recommends tesseract-ocr tesseract-ocr-tur \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY matchwatch_ai ./matchwatch_ai
RUN useradd --system --no-create-home app
USER app
EXPOSE 8090
CMD ["uvicorn", "matchwatch_ai.app:factory", "--factory", "--host", "0.0.0.0", "--port", "8090"]
