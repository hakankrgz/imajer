# Plan Uygunluk ve Eksik Analizi

Tarih: 2026-07-29
Revizyon: 10 — taşınabilir masaüstü paketleri ve okunabilir PDF raporu
Doğrulanan geliştirme sürümü: `0.6.8`

## 1. Güncel sonuç

Proje artık yalnız mimari bir prototip değildir. Yerel controller ve ayrı agent
binary'leriyle uçtan uca çalışan disk edinimi; framed in-memory streaming,
chunk doğrulaması, parçalı RAW yazımı, bağımsız yeniden hash, rapor, canonical
imzalı evidence index ve sonradan doğrulama akışlarıyla tamamlanmıştır.

`0.3.0` ile localhost'a kapalı, CSRF token korumalı grafik web arayüzü
eklenmiştir. Yeni vaka oluşturma, mevcut job ile discover/acquire/resume,
rapor/cleanup, kanıt doğrulama ve canlı işlem kaydı tarayıcıdan yönetilebilir.

`0.4.0` ile arayüz gerçek masaüstü paketine dönüştürülmüştür. Apple Silicon ve
Intel Mac için ad-hoc imzalı `.app`; Windows x64 ve ARM64 için konsol
göstermeden çalışan `.exe` paketleri üretilmektedir. Uygulama çift tıklamada
arayüzü açar, paket içindeki doğru agent'ı seçer, kullanıcı verisini işletim
sisteminin uygulama veri dizininde tutar ve ilk açılışta güvenli izinli Ed25519
incelemeci anahtarını üretir. Aynı uygulamanın ikinci kez açılması yeni sunucu
başlatmak yerine çalışan arayüzü öne getirir.

`0.5.0` ile macOS'ta varsayılan tarayıcı açma kaldırılmış, AppKit içindeki
native `WKWebView` penceresine geçilmiştir. Go controller aynı `.app` içinde
ayrı `imajer-core` süreci olarak çalışır; pencere kapanınca arka uç da kontrollü
biçimde sonlandırılır. Windows'ta controller, Microsoft Edge'i normal tarayıcı
sekmesi olarak değil; ayrı kullanıcı profiline sahip, sekmesiz ve adres
çubuksuz uygulama penceresi olarak başlatır. Pencere kapanınca yerel servis de
kapanır.

`0.6.0` ile masaüstü arayüzünde disk yolu, seri/ID, model, byte cinsinden
boyut, sektör boyutu ve hedef mimarisine uygun agent seçimini kullanıcının
elle girmesi zorunluluğu kaldırılmıştır. Uygulama, görüntü/staging veya agent
yüklemeden önce SSH üzerinde `lsblk`, WinRM HTTPS üzerinde salt-okunur CIM
sorgusu çalıştırır; hedef hostname/OS/mimari/yetki durumunu ve fiziksel diskleri
model, sabit ID, boyut ve cihaz yoluyla listeler. Operatör fiziksel diski açıkça
bu listeden seçer. Bağlantı bilgisi değişirse eski envanter geçersiz sayılır.
Yerel test dosyasının kimliği, boyutu ve sektör varsayımı da otomatik
oluşturulur. Agent, imza ve manifest yolları uzman ayarlarına taşınmış ve
paketten otomatik seçilir. Keşif yanıtındaki hedef metinleri arayüze HTML
olarak işlenmez; yalnız güvenli metin düğümleri kullanılır.

Paketli uygulama ayrıca kendine ait `0600` izinli SSH `known_hosts` güven
deposunu otomatik oluşturur. İlk bağlantıda sunucunun SHA-256 fingerprint'i
operatöre gösterilir; bağımsız kaynaktan doğrulama onayı verilmeden anahtar
kaydedilmez ve SSH oturumu kurulmaz. Daha önce kayıtlı anahtar değişmişse
otomatik güncelleme yapılmaz, bağlantı olası saldırı/anahtar değişimi olarak
durdurulur.

`0.6.1` ile masaüstü canlı kayıt alanı genişletilmiş, yazı boyutu ve satır
aralığı artırılmış ve tam pencereye yakın büyütme modu eklenmiştir. Ayrı sonuç
paneli işlem sürüyor/başarılı/başarısız/iptal durumlarını renk ve açık Türkçe
metinle gösterir. Başlangıç, bitiş ve toplam süre ayrı alanlardır. Edinim
sonunda doğrulama adımı; verify sonunda imaj bütünlüğü ve imza sonucu açıkça
gösterilir. Arka uç her işlem sonuna makine çıktısından bağımsız `SONUÇ`,
UTC bitiş zamanı ve toplam süre satırları ekler.

`0.6.2` ile yerel dosya ve klasör yolu isteyen alanlara macOS ve Windows native
seçim penceresi bağlanmıştır. Edinim/verify sonucunda artifact, byte, chunk,
segment ve oturum sayıları; uzak agent session SHA-256, yerel controller
session SHA-256, bağımsız yerel mantıksal SHA-256 ve Merkle root ayrı bir
bütünlük panelinde gösterilir. Kesintisiz edinimde uzak tam akış ile bağımsız
yerel yeniden okuma açıkça karşılaştırılır. Resume edilmiş disklerde tek uzak
tam kaynak hash'i varmış gibi gösterilmez; her oturum ayrı karşılaştırılır.

Aynı sürümde Linux envanteri root yanında `sudo -n` yetkisini de tanır.
Raspberry Pi SD/eMMC hedefleri için `mmcblk` path fallback'i, CID/name kimliği,
bağlı bölüm uyarısı ve ARM64 RAM provider uyarıları eklenmiştir. `/dev/shm`
`noexec` ise agent hash doğrulamasından sonra gerçek çalıştırma testi başarısız
olur, dosya temizlenir ve `/tmp` adayına geçilir.

`0.6.5` ile yeni bir artifact için boş olmayan kanıt dizinine yazma
reddedilerek eski RAW segmentlerinin yeni edinime karışması engellenmiştir.
Kanıt segmentleri ve rapora girecek dosyalar symlink/özel dosya ise reddedilir.
`verify`, yerel yol ve boyutları etkileyen state alanlarını okumadan önce imzalı
evidence index'i doğrular; ardından artifact kimliği, durum, boyut, offset ve
hash alanlarına sınır denetimi uygular. Retry değerleri sınırlanmış, SSH agent
socket'i tüm kapanış yollarında kapatılmış ve Windows masaüstü iptali doğrudan
process kill yerine çocuğun cleanup/rapor akışını çalıştıran kontrollü iptal
sinyaline geçirilmiştir.

Aynı sürümde resmî AVML 0.20 `amd64` ve `arm64` binary'leri sabit SHA-256 ile
paketlenir. RAM verisi AVML'den agent'a hedefin yalnız loopback arayüzündeki TCP
soketiyle aktarılır; `/dev/stdout` sembolik bağlantısına veya hedefte RAM imaj
dosyasına ihtiyaç duyulmaz. Uzun yerel hash/rapor aşamasından sonra cleanup
öncesi doğrulanmış transport yeniden kurulur. Üç sıfır-ofset RAM attempt'i de
başarısız olursa son provider hatası genel hata içinde korunur.

`0.6.6` ile PDF raporundaki ham `lsblk`/CIM JSON dökümü fiziksel disklerin
okunabilir özetine çevrilmiş, Türkçe karakter desteği ve kontrol karakteri
temizliği eklenmiştir. Masaüstü build'leri en geniş işlemci uyumluluğu için
`GOAMD64=v1` ve `GOARM64=v8.0` tabanında sabitlenmiştir. Temiz kaynak
checkout'unda `make build` artık `dist` dizinini kendisi oluşturur. macOS
paketinin minimum sistemi Go 1.26 çalışma zamanı ile uyumlu biçimde macOS 12
olarak düzeltilmiş; Windows 10/Server 2016 minimumu ve Windows'un Edge/
PowerShell sistem bileşenleri açıkça belgelenmiştir.

Bu sertleştirmeler birim ve regresyon testleriyle; `go test ./...`,
`go test -race ./...`, `go vet ./...`, reproducible/cross build, protokol fuzz,
2 TiB mantıksal Merkle benchmark'ı, `govulncheck` ve imzalı yerel E2E
edinim/verify akışıyla doğrulanmıştır. Windows yolu çapraz derlenmiştir; gerçek
Windows masaüstü iptal kabul testi laboratuvar matrisinde kalır.

İlk analizdeki beş P0 maddeden dördünün kod tarafı kapatılmıştır:

- Gerçek transport nesnesini yeniden oluşturan reconnect ve disk offset-resume.
- İmaj boyutundan bağımsız segment yazıcısı, streaming chunk audit ve Merkle.
- Crash sonrasında doğrulanmış chunk journal'dan state ileri kurtarma.
- Paket bütünlüğü ile tamamlanmış acquisition doğrulamasının ayrılması.

Kalan tek P0, ayrıcalıklı uzak sistem ve gerçek filesystem kabul matrisidir.
Bu matrisin Linux arm64 üzerinde gerçek TCP/SSH/SFTP ve regular-file streaming
alt maddesi 2026-07-28 tarihinde Ubuntu 24.04 ARM64 hedef, root olmayan kullanıcı
ve parolasız sudo ile başarıyla çalıştırıldı. Değişmiş SSH host-key ve yanlış
disk ID reddi, signed agent, doğrudan streaming, üç seviyeli hash doğrulaması
ve uzak cleanup aynı koşuda geçti.
Raw block device, RAM provider, bağlantı kopması ve diğer işletim sistemi/dosya
sistemi varyantları hâlâ bekliyor.
Bu madde kaynak kodla kapatılamaz; yetkili Linux/Windows VM'leri, raw disk,
kernel module/driver ve NTFS/exFAT laboratuvarı gerektirir. Bu nedenle sürüm
çalışan bir geliştirme/laboratuvar sürümüdür; söz konusu matris tamamlanmadan
mahkemeye sunulacak üretim aracı olarak onaylanmamalıdır.

Durum etiketleri:

- **Tamamlandı:** Kod, otomatik test ve uygun olduğunda yerel E2E mevcut.
- **Kısmi:** Çalışan ana yol var; planın belirli bir varyantı veya harici
  doğrulaması eksik.
- **Laboratuvar gerekli:** Kod hazır olsa da ayrıcalıklı gerçek hedef testi yok.
- **Yapılmadı:** İstenen özel bileşen henüz yok.

## 2. Bu revizyonda tamamlanan kritik işler

### P0-01 — Transport reconnect ve dayanıklı resume

**Durum: Tamamlandı; kesintisiz gerçek SSH akışı doğrulandı, ağ-kopması
laboratuvar testi bekliyor.**

- İlk bağlantı `max_attempts` ve 1–30 saniye exponential backoff ile kuruluyor.
- Disk session hatasında eski SSH/WinRM transport kapatılıp yeni transport
  nesnesi oluşturuluyor.
- Yeni session yalnız son `fsync` edilmiş, chunk journal'a yazılmış ve state'e
  geçirilmiş ofsetten başlıyor.
- Retry bütçesi acquisition geneline değil aynı doğrulanmamış ofsete uygulanıyor.
  İleri doğrulanmış ilerleme olursa yeni chunk/ofset için bütçe sıfırlanıyor.
- RAM bağlantı hatasında eski attempt `incomplete` kalıyor ve yeni attempt
  sıfır ofsetten başlıyor.

Kod: `internal/controller/controller.go`
Testler: disk resume, çoklu kesinti, RAM zero-offset restart, ACK replay.

### P0-02 — Sabit bellek ve sınırlı açık dosya

**Durum: Tamamlandı.**

- `SegmentedWriter` aynı anda yalnız bir RAW segmentini açık tutuyor.
- Üretim doğrulama yolu chunk kayıtlarını slice'a yüklemiyor; JSONL streaming
  taranıyor.
- Merkle accumulator yalnız 64 sabit SHA-256 node'u tutuyor.
- Bir chunk doğrulama buffer'ı dışında veri boyutuyla büyüyen yapı yok.
- 2 TiB mantıksal imajı temsil eden 262.144 adet 8 MiB Merkle leaf benchmark'ı
  tek iterasyonda yalnız `288 B/op`, `5 allocs/op` sonucu verdi. Bu ölçüm Merkle
  bileşenine aittir; tam process RSS kabul testi yine laboratuvar matrisindedir.

Kod: `internal/evidence/state.go`
Test/benchmark: `internal/evidence/state_test.go`.

### P0-03 — Crash journal uzlaştırması

**Durum: Tamamlandı.**

Başlangıçta RAW baytları, `chunks.jsonl` ve `state.json` birlikte denetlenir.
Chunk journal state'in ilerisindeyse ve RAW hash'leri doğru/kesintisiz ise state
güvenle ileri alınır ve `state_recovered` olayı yazılır. State journal'ın
ilerisindeyse acquisition durdurulur.

### P0-04 — `verify` sonuç semantiği

**Durum: Tamamlandı.**

Komut artık ayrı sonuçlar üretir:

- `ACQUISITION_VERIFIED`
- `NON_EVIDENTIARY_PARTIAL`
- `PACKAGE_INTEGRITY_OK`

Hiç tamamlanmış acquisition yoksa veya failed/running artifact varsa non-zero
sonuç döner. Tamamlanmış RAM attempt'inin yanında korunan partial attempt'ler
başarıyı yanlış biçimde engellemez.

### P0-05 — Ayrıcalıklı entegrasyon matrisi

**Durum: Kısmi — Linux arm64 regular-file ile AWS Linux amd64 AVML/raw EBS
E2E geçti; matrisin kalanı laboratuvar gerekli.**

Eksik yürütmeler:

- Linux arm64 üzerinde gerçek raw block device (regular-file akışı geçti).
- Windows Server 2019/2022/2025 WinRM HTTPS VM.
- Hedef-kernel LiME ve WinPmem ayrıcalıklı edinimleri.
- APFS dışında NTFS ve exFAT kanıt deposu.
- Ağ fault proxy ile deterministik TCP/SSH/WSMan kopma matrisi.

Bu sonuçlar sürüm, hedef build/kernel, kullanılan tool hash'i ve test kanıt
paketiyle ayrıca arşivlenmelidir.

Tamamlanan Linux arm64 SSH testi:

- Hedef: Alpine 3.22, Linux `6.12.76-linuxkit`, arm64, root SSH oturumu.
- Transport: `127.0.0.1:22222` üzerinden gerçek SSH ve SFTP.
- Host key: doğrulanmış `known_hosts`; yalnız Ed25519 test kullanıcı anahtarı.
- Agent: signed-tool manifest doğrulaması, SFTP upload, uzak SHA-256 kontrolü
  ve edinim sonrası cleanup.
- Kaynak: hedefte salt-okunur 24 MiB `/evidence/source.raw`.
- Akış: üç adet 8 MiB chunk, 16 MiB + 8 MiB iki RAW segment.
- Sonuç: `verified_continuous`, retry `0`, session `1`.
- Uzak stream, yerel mantıksal ve bağımsız yeniden okuma SHA-256:
  `95aeaae03b56c171cf88753c821630a3c24f1fcf406cec3e17d56781aa3f8369`.
- İmzalı evidence index doğrulaması: başarılı.
- Hedef `/dev/shm` ve `/tmp` geçici agent/staging kalıntısı: yok.
- Kanıt paketi secret taraması: başarılı.

Tekrarlanabilir düzenek: `test/remote-ssh/run-test.sh`
Arşivlenmiş özet: `test/remote-ssh/TEST_SONUCU.md`

Tamamlanan AWS Linux amd64 testi:

- Hedef: AWS Linux `amd64`, kernel `7.0.0-1006-aws`, parolasız sudo.
- RAM: imzalı resmî AVML 0.20, loopback-only TCP streaming, hedefte RAM imaj
  dosyası yok; `1.023.342.556` byte, `verified_continuous`, retry `0`.
- Disk: 8 GiB Amazon EBS raw block device, native read-only provider.
- İki gerçek bağlantı kesintisinden sonra doğrulanmış
  `2.332.033.024` ve `7.038.042.112` byte ofsetlerinden devam edildi.
- Sonuç: disk `chunk_verified_composite`, 1024 doğrulanmış 8 MiB chunk,
  3 session, retry `2`; imzalı evidence index `PACKAGE_INTEGRITY_OK`.
- Son çalışmada geçici AVML, agent ve case marker otomatik kaldırıldı.
- Birden fazla `both` çalışması her seferinde yeni, kesintisiz ve doğrulanmış
  RAM snapshot'ı üretirken mevcut disk state'ini kaldığı ofsetten sürdürdü.

## 3. Tamamlanan plan maddeleri

### Streaming, protokol ve resume

- Hedefte imaj/staging oluşturmayan doğrudan stdout/ağ akışı.
- Varsayılan 8 MiB mantıksal chunk.
- WinRM için 64 KiB uygulama-seviyesi sub-frame; frame başına SHA-256/ACK ve
  controller'da doğrulanmış 8 MiB yeniden birleştirme.
- Frame kimliği, session, sequence, offset, UTC read time, provider ve hash.
- Logical chunk ancak tüm sub-frame'ler ve logical SHA-256 eşleştikten sonra
  RAW'a yazılıyor.
- Commit sırası: RAW write+`fsync` → chunk JSONL+`fsync` → atomic state →
  ACK.
- ACK kaybında tekrar gelen chunk yazılmadan yerel kanıt ve journal üzerinden
  idempotent doğrulanıyor.
- Disk resume öncesinde path/ID, model (job'da verilmişse), toplam boyut ve
  sektör boyutu karşılaştırılıyor.

### Bütünlük ve kanıt paketi

- Uzak ve yerel chunk SHA-256 karşılaştırması.
- Her kesintisiz session için uzak/yerel stream SHA-256.
- Yerel mantıksal birleşimin bağımsız yeniden okuma SHA-256 değeri.
- Sıralı domain-separated streaming Merkle root.
- `verified_continuous`, `chunk_verified_composite`, `incomplete`, `failed`
  durumları.
- Completed ve incomplete/failed artifact'ler için
  `artifact-manifest.json`.
- 2 GiB'ye kadar `disk.001` / `memory.001` segmentleri.
- RFC3339Nano JSONL olay ve session kayıtları.
- Evidence tree içindeki dosyaların boyut/SHA-256 listesini içeren
  `evidence-index.json`.
- Evidence index'in diskteki exact compact byte dizisi imzalanıyor; verifier
  canonical olmayan eşdeğer JSON'u dahi reddediyor.
- Harici PKCS#8 Ed25519 anahtar ve detached signature.
- Harici trusted public key ile doğrulama.

### Güvenlik ve footprint

- SSH `known_hosts` zorunluluğu.
- WinRM'de yalnız HTTPS; Basic, NTLM ve Kerberos desteği.
- Windows geçici case dizininde protected ACL; current identity, SYSTEM ve
  Administrators dışındaki miras kaldırılıyor.
- Linux upload dizini `0700`.
- Uploaded ve preinstalled agent için signed manifest + transport üzerinden
  bağımsız SHA-256 kontrolü.
- Uploaded tool'lar için signed manifest, OS/arch/kernel eşleşmesi ve hedefte
  yeniden SHA-256.
- Preinstalled agent durumunda da case marker kuruluyor.
- Cleanup yalnız yereldeki case/evidence marker ile uzak marker SHA-256
  eşleşirse çalışıyor.
- Preinstalled agent marker'ı tool'ları koruyor fakat agent'ı silmiyor.
- Cleanup, iptal edilmiş edinim context'inden bağımsız ve varsayılan 2 dakika
  timeout ile deneniyor.
- Kritik state/session/event yazım hataları başarı yolunda yutulmuyor.
- Redaction; password, passphrase, secret, token, API key, authorization,
  credential, bearer, nested map ve slice değerlerini kapsıyor.

### Rapor ve CLI

- Türkçe PDF'de profil, hedef OS/mimari/kernel/saat/RAM/storage, yerel
  filesystem, kaynak path/ID/model/boyut/sektör, tool sürüm/hash/lisans/trust,
  artifact zamanları, retry, session zaman/ofset/hash/hata, footprint ve
  uyarılar bulunuyor.
- `report` komutu önceki footprint, warnings ve tool envanterini koruyor;
  session JSONL kayıtlarını yeniden yüklüyor.
- Progress: byte/yüzde, anlık ve ortalama hız, ETA, toplam retry, ofset ve
  doğrulama seviyesi.
- YAML üzerine profile/output/signing key/host/port/user/password-env,
  disk path/ID/model/size/sector/provider ve RAM provider flag override'ları.
- Flag ile tamamlanacak eksik disk alanları için YAML validation override
  sonrasına erteleniyor.
- Disk wizard'da path, seri/ID, model, boyut ve sektör açıkça soruluyor.

### Build ve otomatik doğrulama

- Go 1.26.5, CGO kapalı build.
- Controller: darwin amd64/arm64, windows amd64/arm64.
- Agent: darwin amd64/arm64, linux amd64/arm64, windows amd64/arm64.
- Son kullanıcı paketleri: macOS Apple Silicon/Intel ve Windows x64/ARM64.
- macOS `.app` bundle kimliği, plist, özel klasik ikon, ad-hoc code signature
  ve paket sonrası `codesign --verify --deep --strict` doğrulaması.
- macOS Apple Silicon ve Intel için Swift/AppKit + `WKWebView` native pencere
  kabuğu; tarayıcı uygulaması başlatılmıyor.
- Kurulu `/Applications/IMAJER.app` üzerinde CoreGraphics ile ekranda
  `IMAJER — Adli İmaj Alma` adlı 1240×852 pencere, `0.5.0` health cevabı ve
  kapatma sonrası hem pencere hem `imajer-core` sürecinin sonlandığı doğrulandı.
- `0.6.0` paketi `/Applications/IMAJER.app` üzerine kuruldu; ad-hoc kod imzası,
  `0.6.0` health cevabı ve 1240×852 native pencere doğrulandı. Arayüz API'siyle
  yalnız kaynak yolu verilerek yapılan 2 MiB yerel edinimde disk ID/model/size/
  sector otomatik üretildi; kaynak ve `disk.001` SHA-256 eşleşti,
  `verified_continuous`, `ACQUISITION_VERIFIED` ve `PACKAGE_INTEGRITY_OK`
  sonuçları alındı.
- `0.6.1` paketi aynı native pencerede gerçek yerel edinim ve verify işlemiyle
  sınandı. Sonuç panelinde başlangıç/bitiş/süre, “İmaj alma tamamlandı” sonraki
  adım açıklaması ve verify sonrasında “Kanıt başarıyla doğrulandı”,
  “İMAJ DOĞRULANDI” ile “İMZA GEÇERLİ” rozetleri görsel olarak doğrulandı.
  Canlı kaydın büyük yazısı, 420 px varsayılan yüksekliği, satır sayacı ve
  kalıcı `SONUÇ`/bitiş/süre satırları doğrulandı.
- `0.6.2` Ubuntu 24.04 ARM64 SSH/SFTP E2E testinde root olmayan `forensic`
  hesabı ve `sudo -n` ile çalıştı. Değişmiş host-key ile yanlış disk kimliği
  reddedildi. Üç adet 8 MiB chunk sonunda uzak ve yerel SHA-256
  `95aeaae03b56c171cf88753c821630a3c24f1fcf406cec3e17d56781aa3f8369`
  olarak eşleşti; `verified_continuous`, `PACKAGE_INTEGRITY_OK` ve geçici uzak
  dosya kalıntısı olmaması doğrulandı. Test container'ı otomatik kaldırıldı.
- Windows masaüstü binary'lerinde GUI subsystem; çift tıklamada konsol
  penceresi veya normal tarayıcı sekmesi açılmıyor.
- Altı agent binary'si Ed25519 imzalı tool manifestinde; paket içindeki public
  trust key ile manifest ve tüm artifact SHA-256 değerleri doğrulandı.
- Dört dağıtım ZIP'i `SHA256SUMS` ile doğrulandı.
- Bu Mac'te Finder `open IMAJER.app` eşdeğeri gerçek açılış, localhost health,
  paket yolu seçimi, otomatik incelemeci anahtarı ve güvenli kapanış geçti.
- `go test ./...`, `go test -race ./...` ve `go vet ./...` geçti.
- `govulncheck v1.6.0` ile çağrılabilir güvenlik açığı bulunmadı. İlk taramada
  bulunan GO-2026-5543, `github.com/Azure/go-ntlmssp v0.1.1` yükseltmesiyle
  kapatıldı.
- CI frame fuzz smoke, 2 TiB Merkle benchmark, govulncheck, deterministic
  double-build karşılaştırması ve cross-build çalıştırıyor.
- Yerel gerçek-binary E2E:
  - 2 MiB synthetic source.
  - Controller, ayrı agent process'inden iki 1 MiB chunk aldı.
  - Kaynak SHA-256 ile `disk.001` SHA-256 aynı:
    `46a057e18618cfd3c0f15a1591ca151ded1138a72e9d307da220cac91b0530e3`.
  - Sonuç `verified_continuous`.
  - Harici public key ile `PACKAGE_INTEGRITY_OK`.

## 4. Kalan işlevsel eksikler

### P1-01 — Özel FTK provider adapter'ı

**Durum: Yapılmadı.**

Proprietary FTK binary dağıtılmıyor. İmzalı generic range/stdout streaming
adapter sözleşmesi mevcut, ancak FTK'nin belirli sürümlerine ait registry,
komut satırı adapter'ı ve otomatik fallback yoktur. Dosya-destination kullanan
FTK modu zero-image-footprint kuralına aykırı olduğu için etkinleştirilmemelidir.

### P1-02 — Tek job içinde bağımsız AVML ve LiME aday listesi

**Durum: Kısmi.**

AVML ve LiME provider kodları çalışır; `.ko` verilmiş auto senaryosunda RAM yeni
attempt'te LiME'ye geçebilir. Buna karşın aynı job içinde ayrı signed AVML
binary ve signed LiME modülünü sıralı provider nesneleri olarak tanımlayan
genel provider registry yoktur.

### P1-03 — Tüm target-PATH araçlarında signed-tool zorlaması

**Durum: Kısmi.**

Controller'ın upload ettiği her agent/tool zorunlu olarak imzalıdır. Hedef
işletim sisteminin `dd`/`dc3dd` binary'si ise probe envanterinden seçilebilir ve
release tool manifestine tabi değildir. Embedded WinPmem driver'ın exact
SHA-256/AuthentiCode bilgisi de controller raporuna ayrı tool kaydı olarak
taşınmamaktadır.

Gerekli politika kararı: işletim sistemi trusted computing base içindeki
binary'ler kabul edilecekse path/version/hash raporlanmalı; kabul edilmeyecekse
`auto` yalnız signed uploaded tool veya native reader seçmelidir.

### P1-04 — PDF'de gerçek Türkçe Unicode font

**Durum: Kısmi.**

Rapor içeriği Türkçedir fakat gömülü Unicode font olmadığı için PDF üreticisi
Türkçe karakterleri ASCII'ye translitere eder. UTF-8 destekli lisanslı font
asset'i gömülmelidir.

### P1-05 — Yapılandırılmış custody transfer kayıtları

**Durum: Kısmi.**

Examiner, kurum, yetki referansı, acquisition timeline ve integrity kayıtları
vardır. Teslim eden/alan, zaman, lokasyon, amaç, mühür ve custody imza geçmişi
için ayrı append-only veri modeli henüz yoktur.

## 5. Kalan test, UX ve supply-chain işleri

### P2-01 — Discovery tabanlı interaktif disk seçimi

**Durum: Masaüstü arayüzünde tamamlandı; CLI wizard için yapılmadı.**

Masaüstü uygulaması hedefe salt-okunur SSH/WinRM sorgusuyla bağlanır ve
fiziksel diskleri açılır listede model, sabit ID, boyut ve cihaz yoluyla
gösterir. Operatör listeden açık seçim yapar; disk hiçbir zaman otomatik
seçilmez. Seçim job içindeki path/ID/model/size/sector alanlarını otomatik
doldurur ve hedef mimarisine uygun imzalı agent yolunu seçer. Standart
`~/.ssh/known_hosts`, paket içi agent/manifest/güven anahtarı ve uygulamanın
incelemeci imza anahtarı kullanıcıdan gizli varsayılanlar olarak atanır.

CLI `wizard` hâlâ disk değerlerini metin olarak sorar; bu eksik yalnız CLI
etkileşimli kullanımını etkiler.

### P2-02 — Eksik argümanda aynı komut içinde wizard

`wizard` ayrı ve çalışır bir komuttur. `acquire --job` eksikse otomatik olarak
aynı süreçte wizard açmak yerine kullanıcıyı wizard'a yönlendirir.

### P2-03 — SHA-256 acceleration telemetry

Go `crypto/sha256`, desteklenen mimaride runtime-optimized implementasyonu
seçer. Kullanılan CPU capability/assembly path'i ölçüp raporlayan ayrı telemetry
yoktur.

### P2-04 — Fault/fuzz kapsamı

Mevcut:

- Tam chunk sonrası kesinti ve disk resume.
- Aynı acquisition içinde çoklu ofsette retry bütçesi.
- ACK replay/idempotency.
- RAM zero-offset restart.
- State'in doğrulanmış journal'dan ileri kurtarılması.
- WinRM sub-frame reassembly.
- Bir byte yerel bozulma.
- Incomplete manifest.
- Nested secret redaction.
- Frame parser fuzz.

Kalan:

- Gerçek socket üzerinde frame ortası kopma ve reconnect.
- Gerçek timeout.
- Disk identity'nin retry arasında değişmesi.
- Gerçek disk-full/fsync failure.
- Agent kill ve bütün crash commit noktaları.
- Cleanup marker mismatch/idempotency fault matrisi.
- Offset/state/redaction/cleanup için ek fuzz hedefleri.

### P2-05 — Filesystem ve native runtime matrisi

- APFS yerel regular-file E2E tamamlandı.
- NTFS/exFAT gerçek büyük segment, disk-full, fsync ve atomic rename testi yok.
- Apple Silicon masaüstü `.app` bu Mac'te native ve gerçek çift tıklama
  akışıyla doğrulandı.
- Intel Mac ile Windows x64/ARM64 paketleri doğru native executable
  mimarisinde üretildi; bu diğer işletim sistemlerinde gerçek açılış sonucu
  hâlâ laboratuvar maddesidir.

### P2-06 — Lisans/SBOM/provenance ve platform imzalama

CI'da sabitlenmiş `govulncheck v1.6.0` taraması ve deterministic double-build
vardır. Agent paketinin Ed25519 tool manifesti ve dağıtım ZIP checksum'ları
vardır. Dependency license scanner, SBOM, SLSA/provenance, Apple Developer ID
notarization ve Windows Authenticode imzalama henüz yoktur.

## 6. Güncel kabul kararı

### Çalışan ve kullanılabilir kapsam

- Yetkili laboratuvar/development ortamında local regular-file disk acquisition.
- İzole Linux arm64 hedefinde gerçek SSH/SFTP regular-file disk acquisition.
- AWS Linux amd64 hedefinde AVML RAM ve raw EBS disk acquisition.
- Gerçek SSH/ağ kesintilerinden disk ofset-resume ve sonrasında tam cleanup.
- Kanıt formatı, resume algoritması, chunk/session/logical bütünlük modeli.
- Native controller/agent build'leri.
- İmzalı evidence package oluşturma ve bağımsız doğrulama.
- SSH transport ve güvenlik katmanının Linux arm64 kesintisiz E2E doğrulaması.
- WinRM için uygulanmış fakat gerçek Windows hedefte henüz doğrulanmamış katman.

### Üretim/adli kullanım öncesi zorunlu kapanış

1. P0-05 uzak VM/raw disk/RAM provider/filesystem matrisi.
2. Kurumun signed target-tool/OS-TCB politika kararı ve uygulanması.
3. Kullanılacaksa sürüm-spesifik FTK streaming adapter doğrulaması.
4. Custody transfer şeması ve Unicode Türkçe PDF fontu.
5. Release SBOM, lisans kontrolü, provenance ve imzalı dağıtım.

Karar:
**“Çalışan geliştirme/laboratuvar sürümü — ayrıcalıklı kabul matrisi
tamamlanmadan mahkeme/üretim onayı verilmemeli.”**
