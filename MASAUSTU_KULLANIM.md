# IMAJER 0.6.6 Masaüstü Kullanımı

IMAJER masaüstü paketi Terminal veya YAML kullanmadan, çift tıklayarak
çalıştırılabilir. Safari, Chrome veya normal bir tarayıcı sekmesi açılmaz.
Arayüz IMAJER'in kendi penceresinde gösterilir. Yerel arka uç yalnız bu
bilgisayardaki `127.0.0.1` adresini dinler; internete yayınlanmaz.

Yayınlanan ZIP paketi son kullanıcı bilgisayarında Go, Node.js, Python veya
ayrı AVML kurulumu istemez. Minimum istemci sistemi macOS 12 Monterey ya da
Windows 10/Windows Server 2016'dır.

## Bu Mac'te

1. `IMAJER.app` dosyasını **Uygulamalar** klasörüne taşıyın.
2. Uygulamalar içinden **IMAJER** simgesine çift tıklayın.
3. Açılan IMAJER penceresinden **Yeni işlem oluştur** bölümünü seçin.
4. Hedef türünü seçin:
   - Linux sunucu için **SSH**
   - Windows sunucu için **WinRM HTTPS**
   - Bu bilgisayardaki sıradan bir test dosyası için **Yerel**
5. Uzak hedefte IP, kullanıcı ve parolayı girip **Bağlan ve diskleri getir**
   düğmesine basın.
6. Linux sunucuya ilk bağlantıysa gösterilen SSH fingerprint'i sunucu
   yöneticisinden doğrulayın; eşleşiyorsa **Doğruladım, güven ve bağlan**
   düğmesine basın.
7. Programın bulduğu disklerden doğru olanı model, ID ve boyutuna bakarak
   seçin. `/dev/sda` veya `PhysicalDrive0` gibi bir yolu ezberlemeniz gerekmez.
8. **Tüm disk**, **Canlı RAM** veya **RAM + Disk** seçimini yapın.
9. Önce **Bilgileri kontrol et**, sonra **İmajı başlat** düğmesine basın.
10. İşiniz bittiğinde sağdaki **Uygulamayı kapat** düğmesini kullanın.

Kanıtlar varsayılan olarak `Belgeler/IMAJER-Evidence` dizinine yazılır. Uygulama
ilk açılışta incelemeci Ed25519 imza anahtarını
`~/Library/Application Support/IMAJER/keys` altında otomatik oluşturur.

Linux `amd64` ve `arm64` paketlerinde imzalı Microsoft AVML 0.20 hazır gelir;
ayrıca AVML kurmanız gerekmez. RAM verisi hedefte yalnız loopback
`127.0.0.1` üzerinden agent'a aktarılır ve hedef diskte RAM imajı oluşturulmaz.

## Başka bir Mac'te

- M1, M2, M3, M4 veya sonraki Apple işlemcileri için
  `IMAJER-macOS-Apple-Silicon-*.zip` paketini kullanın.
- Intel işlemcili Mac için `IMAJER-macOS-Intel-*.zip` paketini kullanın.

ZIP'i açın ve `IMAJER.app` dosyasını Uygulamalar klasörüne taşıyın. Paket henüz
Apple Developer ID ile notarize edilmediği için macOS ilk açılışta uyarı
gösterebilir. Bu durumda uygulamaya Control-tıklayın, **Aç** seçeneğini seçin
ve bir kez daha **Aç** düğmesine basın.

## Windows'ta

- Çoğu bilgisayar için `IMAJER-Windows-x64-*.zip` paketini kullanın.
- ARM tabanlı Windows bilgisayar için `IMAJER-Windows-ARM64-*.zip` paketini
  kullanın.

ZIP'in tamamını bir klasöre çıkarın ve `IMAJER.exe` dosyasına çift tıklayın.
IMAJER sekmesiz ve adres çubuksuz ayrı bir uygulama penceresinde açılır.
Windows'ta bu pencere yüklü Microsoft Edge motorunun ayrı ve izole bir
uygulama profilini kullanır; normal Edge sekmeleriniz açılmaz. Windows
SmartScreen imzasız uygulama uyarısı gösterirse **Ek bilgi** ve ardından
**Yine de çalıştır** seçilebilir.

Windows masaüstü paketi, Windows ile gelen Microsoft Edge ve Windows
PowerShell bileşenlerini kullanır. Kurum imajında bu bileşenler kaldırılmışsa
arayüz penceresi veya dosya seçici çalışmaz; CLI kullanılabilir ya da sistem
bileşenleri kurum yöneticisi tarafından yeniden etkinleştirilmelidir.

Windows'ta uygulama ayarları ve otomatik oluşturulan incelemeci anahtarı
`%AppData%\IMAJER` altında, kanıtlar varsayılan olarak
`Belgeler\IMAJER-Evidence` altında tutulur.

## Önemli sınırlar

- macOS paketi ad-hoc imzalıdır; başka Mac'lerde uyarısız dağıtım için Apple
  Developer ID imzası ve notarization gerekir.
- Windows paketi Authenticode sertifikasıyla imzalanmadığından SmartScreen
  uyarısı gösterebilir.
- Agent dosyaları paket içindeki Ed25519 imzalı manifest ile uygulama
  tarafından doğrulanır. `tool-release-private.pem` dağıtım paketine girmez.
- Gerçek disk ve RAM edinimi yönetici/root yetkisi ve açık yasal yetki
  gerektirir.
- “Zero disk footprint”, hedefte imaj veya staging parçası oluşturulmaması
  anlamındadır; uzak erişim ve sürücü/agent işlemleri sistem loglarında iz
  bırakabilir.
- Genel `exit status 1` mesajı kök neden değildir. Canlı kayıttaki önceki hata,
  `events.jsonl`, artifact `state.json` ve son **Kanıt doğrula** sonucu birlikte
  değerlendirilmelidir.
- Kesilen disk aynı job ile doğrulanmış ofsetten devam edebilir. RAM devam
  etmez; yeni bir sıfır-ofset attempt olarak yeniden alınır.
