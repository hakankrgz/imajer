# IMAJER Basit Arayüz Kullanımı

## macOS

Finder'da proje klasörünü açın ve şu dosyaya çift tıklayın:

```text
IMAJER-ARAYUZ.command
```

Tarayıcı otomatik açılmazsa Terminal'de gösterilen
`http://127.0.0.1:8765` adresini Safari veya Chrome'a yazın.

## Windows

Şu dosyaya çift tıklayın:

```text
IMAJER-ARAYUZ-WINDOWS.cmd
```

Başlatıcı işlemci mimarisine göre amd64 veya ARM64 controller'ı seçer.

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
- Arayüz sunucusunu tamamen kapatmak için Terminal penceresinde `Ctrl+C`
  kullanın.
