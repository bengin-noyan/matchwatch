# Faz 3 — Web uygulaması: Tasarım

Tarih: 2026-11-05. Durum: mimar kararıyla onaylandı (kullanıcı kararı Claude'a bıraktı; tasarım `frontend-design`
skill'iyle yapıldı). Üst belge: `docs/spec.md` (D13: kullanıcı/login Faz 3). Faz 3 iki parçadır: **3a** gerçek giriş,
**3b** Vue SPA.

## 1. Konu, kullanıcı, birincil iş

- **Konu:** Türk şirketlerinde satın alma faturalarının sözleşme ve irsaliyeyle karşılaştırılması (3-way match).
- **Kullanıcı:** iç denetçi, satın alma/muhasebe uzmanı. Günde onlarca faturaya bakar; Excel ve kâğıt evrakla çalışmaya
  alışkın; sayıya güvenir, süse değil.
- **Birincil iş:** kırmızı faturayı bulmak, farkın kanıtını (sözleşme maddesi ↔ fatura satırı) tek bakışta görmek ve
  karar vermek (itiraz / istisna / açık). İkincil: belge yüklemek, sözleşme taslağını ve kalem önerisini onaylamak.

## 2. Görsel fikir: kırmızı kalem, mavi tükenmez

Denetçinin masasındaki iki kalem: **kırmızı kalem** bulguyu işaretler, **mavi tükenmez** insan kararını yazar. Uygulama
bu ayrımı renk dilinin tek kuralı yapar:

- Sistemin bulduğu her şey (bulgu, fark, beklenmeyen değer) **kırmızı kalem** rengindedir.
- İnsanın verdiği her karar (itiraz, istisna, not, onaylanan eşleşme) **mavi tükenmez** rengindedir.
- Geri kalan her şey mürekkep ve kâğıttır; renk yalnızca bu iki anlamı taşır.

Zemin, muhasebe defterinin hafif yeşil kâğıdıdır (ledger). Bu bir göndermedir, gazete düzeni değildir: ızgara çizgisi
yalnızca gerçekten satır olan yerde (fatura listesi, kalem tabloları) vardır.

### Renk (tokenlar)

| Ad | Hex | Görev |
|---|---|---|
| `paper` | `#EFF3EC` | uygulama zemini (defter kâğıdı) |
| `sheet` | `#FFFFFF` | belge yüzeyi (fatura/sözleşme sayfası) |
| `ink` | `#1D2A23` | metin, yeşile çalan mürekkep |
| `rule` | `#C7D3C4` | satır çizgileri, kenarlıklar |
| `pencil` | `#BE1E2D` | kırmızı kalem: bulgular, farklar, RED durum |
| `ballpoint` | `#2445A0` | mavi tükenmez: insan kararları ve birincil eylemler |
| `amber` | `#8A5C0A` | uyarı (YELLOW, WARNING); tek ikincil anlam |

Yeşil "GREEN" durumu ayrı bir renk almaz: temiz fatura mürekkeple yazılır, üstünde kalem izi yoktur. Kontrast: `ink`,
`pencil`, `ballpoint`, `amber` hepsi `paper` ve `sheet` üstünde WCAG AA (≥ 4.5:1) sağlar.

### Tipografi

- **Atkinson Hyperlegible Next** (Braille Institute; Türkçe glif desteği var): bütün arayüz ve sayılar. Okunabilirlik
  için tasarlanmış, karışan glifleri (0/O, 1/l/I) ayırır; denetimde rakam okumak işin kendisidir. Sayılarda
  `font-variant-numeric: tabular-nums`.
- **Caveat** (el yazısı): yalnızca kalem izleri. Bulgunun kenar notu (ör. "+45,00 TL") kırmızı, insanın notu mavi.
  Hiçbir başlıkta ya da düğmede kullanılmaz.
- Yazı tipleri `@fontsource` ile kendi sunucumuzdan servis edilir (Google Fonts CDN yok: KVKK, kullanıcı IP'si dışarı
  gitmez).
- Ölçek (Bringhurst'ün klasik ölçeği): 12 / 14 / 16 / 18 / 21 / 24 / 36. Gövde 16 px, satır yüksekliği 1.5; tablo 14 px.
  Başlıklar ağırlıkla (600) ayrılır, büyüklükle değil.
- Büyük harfli etiket, başlık üstü küçük etiket, tek kelimeyi renklendirme yok.

### Yerleşim

Masaüstü öncelikli (denetçi büyük ekranda çalışır), telefonda tek sütuna düşer. Metin sola hizalı, sayılar sağa.

```
┌──────────┬──────────────────────────────────────────────────────┐
│ matchwatch│  Faturalar                       [Belge yükle]      │
│           │  4 fatura incelemeni bekliyor; 214,12 TL fark.      │
│ Faturalar │ ┌───────────────────────────────────────────────────┐│
│ Sözleşme  │ │ durum  fatura no   tedarikçi   tarih    tutar  fark││
│ taslakları│ │ ●      ABC2026…    Demir Bağ.  01.03   2.680  ~~214~││
│ Özet      │ │        …                                          ││
│           │ └───────────────────────────────────────────────────┘│
└──────────┴──────────────────────────────────────────────────────┘
```

Fatura detayı, uygulamanın imza ekranıdır (SplitDiffViewer). Her bulgu, sözleşme ile faturayı yan yana koyar; farklı olan
değer kırmızı kalemle daire içine alınır, etkisi kenara el yazısıyla yazılır:

```
  Satır 1 · M8 20mm Inox Civata                               
 ┌ Sözleşme S-2026-001, s. 3 ──────┐ ┌ Fatura satırı 1 ───────────┐
 │ Madde 4.1: M8x20 paslanmaz      │ │ 1.000 ADET                 │   +50,00 TL
 │ civata birim fiyatı 1,25 TL     │ │ net birim  ( 1,30 TL )     │   (el yazısı,
 │ + KDV'dir.                      │ │            ‾‾‾‾‾‾‾‾‾‾      │    kırmızı)
 └─────────────────────────────────┘ └────────────────────────────┘
  [İtiraz et]  [İstisna olarak onayla]  not: ____________________ (mavi)
```

### İlkeler

1. Renk anlam taşır: kırmızı = sistem buldu, mavi = insan karar verdi. Üçüncü bir vurgu rengi yok.
2. Kanıt her zaman yan yanadır; denetçi bulguyu görmek için başka sayfaya gitmez.
3. Sayı önce gelir: tutarlar tabular, sağa hizalı, para birimiyle; kuruş asla yuvarlanmaz.
4. Tek hareket anı: bir bulgu çözülünce kırmızı dairenin üstüne mavi bir onay işareti çizilir (SVG stroke). Başka
   otomatik animasyon yok; `prefers-reduced-motion` ile o da kapanır.
5. Yazı: kullanıcının dili, sade fiil, cümle düzeni. Düğme ne yapacağını söyler ("İtiraz et", "Taslağı onayla").

### Gözden geçirme (jenerik varsayılana karşı)

- İlk taslakta ana sayfa "kurtarılan tutar" büyük rakamı + kartlarla açılıyordu: bu, skill'in andığı varsayılan
  (büyük sayı + küçük etiket + kart seti). **Değişti:** açılış ekranı doğrudan iş listesi; toplam tek bir cümlede.
  Özet ekranı kart değil, para birimine göre bir defter satırı.
- Yeşil defter kâğıdı + ızgara, "broadsheet" varsayılanına kayabilirdi. **Sınır:** ızgara yalnızca tablo satırlarında;
  sayfa düzeni gazete sütunları değil; köşe yarıçapı hiyerarşiye göre değişir (sayfa 10 px, alan 6 px, rozet tam yuvarlak).
- Durum rozetleri için trafik ışığı (kırmızı/sarı/yeşil) varsayılandı. **Değişti:** RED kırmızı kalem dairesi, YELLOW
  amber, GREEN işaretsiz (temiz kâğıt). Anlam aynı, dil özgün.
- Kenar notu el yazısı riskli bir seçim; yalnızca kalem izlerinde kullanılarak sınırlandı.

## 3. Faz 3a — gerçek giriş (backend)

- **R1. Kullanıcılar:** `users(id, tenant_id, email UNIQUE (küçük harf), password_hash, created_at)`, RLS'li. Şifre
  bcrypt (`golang.org/x/crypto/bcrypt`, cost 12). Rol yok (Faz 3'te tek rol; çoklu rol ihtiyaç olunca).
- **R2. Giriş:** `POST /api/v1/auth/login {email, password}`. Kullanıcı e-postayla tenant bilinmeden bulunmalı; RLS'i
  delmeden bunu `matchwatch_owner`'a ait `SECURITY DEFINER` fonksiyonu `auth_lookup(email)` yapar (yalnızca id, tenant,
  hash döner). Yanlış e-posta ve yanlış şifre aynı yanıtı ve yaklaşık aynı süreyi alır (bulunamayan e-postada da sahte
  bcrypt karşılaştırması).
- **R3. Oturum:** JWT (mevcut `auth.Sign`, 8 saat) `HttpOnly; SameSite=Strict; Path=/api` çerezinde (`Secure`,
  `COOKIE_SECURE` ile; varsayılan açık, yerelde kapatılır). Middleware çerezi ya da `Authorization` başlığını kabul eder
  (scriptler ve `cmd/devtoken` çalışmaya devam eder). SPA aynı kökenden servis edildiği için CORS yok; `SameSite=Strict`
  CSRF'e karşı korur. `POST /api/v1/auth/logout` çerezi siler. `GET /api/v1/auth/me` → `{email, tenant_name}`.
- **R4. Hesap açma:** halka açık kayıt yok (B2B). `cmd/onboard -name -vkn -rate -email` tenant + ilk kullanıcıyı açar,
  şifreyi stdin'den okur (komut satırı geçmişine düşmez).
- **R5. Kaba kuvvet:** sınırlama aynı e-posta için 15 dakikada 10 başarısız deneme (bellekte; tek API örneği). Çoklu
  örnekte Postgres'e taşınır (`shortcut:` yorumu).

## 4. Faz 3b — Vue SPA (`web/`)

- Vue 3 + TypeScript + Vite + Tailwind + Pinia + TanStack Table (spec §2). Vue Router.
- Geliştirmede Vite `/api`'yi Go API'ye vekâlet eder. Yayında `web/dist` Go API'nin aynı kökeninden servis edilir
  (`WEB_DIR`; ayarlıysa `/` altında statik dosya + SPA geri dönüşü). Ayrı alan adı, CORS yok.
- Ekranlar:
  1. **Giriş.**
  2. **Faturalar** (AuditGrid): durum, fatura no, tedarikçi, tarih, ödenecek, açık fark; sıralama ve durum filtresi.
  3. **Fatura detayı** (SplitDiffViewer): satırlar; her bulgu için sözleşme ↔ fatura yan yana, kenar notu, karar
     düğmeleri ve not; `UNCONTRACTED_ITEM` önerilerinde "Bu kalemle eşleştir" (D16); "Yeniden denetle".
  4. **Belge yükle:** e-Fatura / e-İrsaliye / alındı XML'i; sözleşme PDF'i → taslak.
  5. **Sözleşme taslağı:** çıkarılan alanlar düzenlenebilir form + her alanın kaynak sayfası ve alıntısı; "Taslağı onayla".
  6. **Özet** (RoiSummaryCards): para birimine göre bulunan / itiraz edilen tutar ve durum dağılımı.
- Para: API'den gelen string decimal ve kuruş integer olduğu gibi kullanılır; gösterim `Intl.NumberFormat('tr-TR')` ile
  kuruş → string bölmesiyle yapılır, `Number` ile para hesabı yapılmaz (spec kuralı 1 ön yüzde de geçerli).
- Testler: Vitest (para biçimlendirme, API istemcisi, kritik bileşenler); uçtan uca kontrol `run` skill'iyle gerçek
  tarayıcıda ekran görüntüsü.

## 5. Hedef dışı (Faz 3 sonrası)

Çoklu rol/izin, kullanıcı davet akışı, şifre sıfırlama e-postası, takma ad listeleme/silme ekranı, mükerrer fatura
semantik karşılaştırması, `FX_RATE_DEVIATION` kuralı, yayın altyapısı (TR bulut hesabı, TLS, yedekleme).
