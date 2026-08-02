# IMAJER 0.6.8 Basit Arayüz Kullanımı

## macOS

Uygulamalar klasöründe şu uygulamaya çift tıklayın:

```text
IMAJER.app
```

Arayüz Safari veya Chrome'da değil, doğrudan IMAJER uygulama penceresinde
açılır.

## Windows

İndirdiğiniz ZIP'i tamamen çıkardıktan sonra şu dosyaya çift tıklayın:

```text
IMAJER.exe
```

x64 ve ARM64 için ayrı hazır paketler bulunur.
IMAJER, sekme ve adres çubuğu olmayan ayrı uygulama penceresi açar.

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

Sağdaki **Canlı kayıt** alanı işlem boyunca ilerlemeyi gösterir. **Büyüt**
düğmesiyle kayıt alanını pencere boyutuna çıkarabilir, **En alta** ile en yeni
satıra dönebilirsiniz. İşlem bittiğinde kayıtların üstündeki sonuç kutusu
başarı, hata veya iptal durumunu; başlangıç/bitiş zamanını ve sonraki adımı
açıkça gösterir.

## Gerçek edinim

1. **Yeni işlem oluştur** sekmesine girin.
2. Vaka ve yetki bilgilerini doldurun.
3. `Linux / SSH` veya `Windows / WinRM` hedefini seçin.
4. Sunucu/IP, kullanıcı ve parolayı girin.
5. **Bağlan ve diskleri getir** düğmesine basın.
6. İlk SSH bağlantısında gösterilen fingerprint'i güvenilir bir kanaldan
   doğrulayıp **Doğruladım, güven ve bağlan** düğmesine basın.
7. Disk profili kullanıyorsanız programın bulduğu fiziksel disklerden doğru
   olanı model, ID ve boyutuna bakarak seçin. Disk yolu elle yazılmaz.
8. `Tüm disk`, `Canlı RAM` veya `RAM + Disk` seçin.
9. **Bilgileri kontrol et** düğmesine basın.
10. Kontrol başarılıysa **İmajı başlat** düğmesine basın.

Bağlantı kesildiyse **Mevcut işlem** sekmesinde arayüzün kaydettiği job
dosyasını seçip **Devam et** düğmesine basın.

Disk doğrulanmış son ofsetten devam eder. RAM devam ettirilemez; her yeniden
başlatmada yeni ve sıfır-ofset bir `memory-attempt-NNN` alınır. Bu nedenle
`RAM + Disk` işlemini disk kesintisinden sonra yeniden çalıştırmak yeni bir RAM
snapshot'ı oluşturabilir.

## Hata mesajını yorumlama

`İşlem başarısız: exit status 1` genel bir kapanış kodudur. Asıl neden için
**Canlı kayıt** alanında bu mesajdan önceki satırlara bakın:

- `network is unreachable`, `unexpected EOF`: bağlantı kesilmiştir; disk için
  aynı job ile **Devam et** kullanılabilir.
- `RAM acquisition failed...`: ilgili `memory-attempt-NNN/state.json`
  içindeki `last_error` incelenmelidir.
- `cleanup ... retained`: kanıt silinmemiştir; hedefteki geçici agent/araç için
  **Temizle** çalıştırılmalıdır.

İşlemin gerçek bütünlük sonucu **Kanıt doğrula** ekranındaki
`ACQUISITION_VERIFIED` ve `PACKAGE_INTEGRITY_OK` kayıtlarıdır.

## Güvenlik

- Uygulama penceresinin arka ucu yalnız `127.0.0.1` üzerinde çalışır ve ağa
  yayınlanmaz.
- Aynı anda yalnız bir işlem başlatılabilir.
- Parola arayüz sürecinin belleğinde tutulur; job, log veya rapora yazılmaz.
- Linux RAM, imzalı AVML 0.20 ile hedefin yalnız `127.0.0.1` arayüzündeki
  geçici TCP soketinden agent'a akar; hedef diskte RAM imajı oluşturulmaz.
- **İşlemi güvenle durdur** düğmesi acquisition sürecine iptal sinyali gönderir
  ve sınırlı cleanup işleminin tamamlanmasını bekler.
- Native uygulama penceresini kapatmak yerel arka ucu da kapatır. Çalışan bir
  edinim varsa önce **İşlemi güvenle durdur** düğmesini kullanın ve cleanup
  sonucunu bekleyin.
- Arayüz sunucusunu tamamen kapatmak için sağdaki **Uygulamayı kapat**
  düğmesini kullanın.

Kurulum, ilk açılış uyarıları ve paket seçimi için
[`MASAUSTU_KULLANIM.md`](MASAUSTU_KULLANIM.md) dosyasına bakın.
