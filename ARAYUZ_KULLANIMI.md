# IMAJER Basit Arayüz Kullanımı

## macOS

Uygulamalar klasöründe şu uygulamaya çift tıklayın:

```text
IMAJER.app
```

Tarayıcı otomatik açılmazsa Safari veya Chrome'a
`http://127.0.0.1:8765` adresini yazın.

## Windows

İndirdiğiniz ZIP'i tamamen çıkardıktan sonra şu dosyaya çift tıklayın:

```text
IMAJER.exe
```

x64 ve ARM64 için ayrı hazır paketler bulunur.

## İlk deneme

Ana sayfadaki **Hazır yerel deneme** kartında:

1. **Kontrol et** düğmesine basın.
2. Sağ tarafta “İşlem başarıyla tamamlandı” yazmasını bekleyin.
3. **İmajı al** düğmesine basın.
4. İşlem bitince **Doğrula** düğmesine basın.

Şu iki kayıt görünüyorsa deneme başarılıdır:

```text
ACQUISITION_VERIFIED
PACKAGE_INTEGRITY_OK
```

## Gerçek edinim

1. **Yeni işlem oluştur** sekmesine girin.
2. Vaka ve yetki bilgilerini doldurun.
3. `Linux / SSH` veya `Windows / WinRM` hedefini seçin.
4. `Tüm disk`, `Canlı RAM` veya `RAM + Disk` seçin.
5. Disk ve çıktı bilgilerini doldurun.
6. **Bilgileri kontrol et** düğmesine basın.
7. Kontrol başarılıysa **İmajı başlat** düğmesine basın.

Bağlantı kesildiyse **Mevcut işlem** sekmesinde arayüzün kaydettiği job
dosyasını seçip **Devam et** düğmesine basın.

## Güvenlik

- Arayüz yalnız `127.0.0.1` üzerinde çalışır.
- Aynı anda yalnız bir işlem başlatılabilir.
- Parola arayüz sürecinin belleğinde tutulur; job, log veya rapora yazılmaz.
- **İşlemi güvenle durdur** düğmesi acquisition sürecine iptal sinyali gönderir
  ve sınırlı cleanup işleminin tamamlanmasını bekler.
- Tarayıcı sekmesini kapatmak acquisition sürecini durdurmaz. İşlem durumunu
  görmek için aynı adresi yeniden açabilirsiniz.
- Arayüz sunucusunu tamamen kapatmak için sağdaki **Uygulamayı kapat**
  düğmesini kullanın.

Kurulum, ilk açılış uyarıları ve paket seçimi için
[`MASAUSTU_KULLANIM.md`](MASAUSTU_KULLANIM.md) dosyasına bakın.
