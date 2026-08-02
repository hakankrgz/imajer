# IMAJER Belgeleri

## Kullanıcı belgeleri

- [`MASAUSTU_KULLANIM.md`](MASAUSTU_KULLANIM.md): macOS ve Windows masaüstü
  paketlerinin kurulumu ve açılması
- [`ARAYUZ_KULLANIMI.md`](ARAYUZ_KULLANIMI.md): grafik arayüzde temel işlemler
- [`EKIP_HIZLI_KULLANIM.md`](EKIP_HIZLI_KULLANIM.md): ekran görüntülü kısa ekip
  rehberi
- [`EKIP_HIZLI_KULLANIM.pdf`](EKIP_HIZLI_KULLANIM.pdf): hızlı rehberin
  paylaşılabilir PDF sürümü
- [`KULLANIM_KILAVUZU.md`](KULLANIM_KILAVUZU.md): kurulum, yapılandırma,
  edinim, doğrulama ve sorun giderme ayrıntıları

## Proje ve doğrulama

- [`PLAN_EKSIK_ANALIZI.md`](PLAN_EKSIK_ANALIZI.md): güncel geliştirme,
  doğrulama ve kabul testi durumu
- [`../test/remote-ssh/TEST_SONUCU.md`](../test/remote-ssh/TEST_SONUCU.md): SSH
  uçtan uca test sonuçları

## Belge üretimi

PDF hızlı rehberini proje kökünde şu komutla yeniden oluşturun:

```sh
node docs/ekip-kullanim/build-pdf.mjs
```

Ekran görüntüsü varlıkları `ekip-kullanim/assets/` altında tutulur.
