# IMAJER Masaüstü Kullanımı

IMAJER masaüstü paketi Terminal veya YAML kullanmadan, çift tıklayarak
çalıştırılabilir. Arayüz yalnız bu bilgisayardaki `127.0.0.1` adresinde açılır;
internete yayınlanmaz.

## Bu Mac'te

1. `IMAJER.app` dosyasını **Uygulamalar** klasörüne taşıyın.
2. Uygulamalar içinden **IMAJER** simgesine çift tıklayın.
3. Varsayılan tarayıcıda açılan ekrandan **Yeni işlem oluştur** bölümünü seçin.
4. Hedef türünü seçin:
   - Linux sunucu için **SSH**
   - Windows sunucu için **WinRM HTTPS**
   - Bu bilgisayardaki sıradan bir test dosyası için **Yerel**
5. Vaka bilgilerini ve hedef bilgilerini doldurup önce **Hedefi kontrol et**,
   sonra **İmajı başlat** düğmesine basın.
6. İşiniz bittiğinde sağdaki **Uygulamayı kapat** düğmesini kullanın.

Kanıtlar varsayılan olarak `Belgeler/IMAJER-Evidence` dizinine yazılır. Uygulama
ilk açılışta incelemeci Ed25519 imza anahtarını
`~/Library/Application Support/IMAJER/keys` altında otomatik oluşturur.

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
Arayüz varsayılan tarayıcıda açılır. Windows SmartScreen imzasız uygulama
uyarısı gösterirse **Ek bilgi** ve ardından **Yine de çalıştır** seçilebilir.

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
