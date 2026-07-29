# imajer

`imajer`, uzak Linux ve Windows sunucularından fiziksel disk veya canlı RAM
verisini hedefte imaj/staging dosyası oluşturmadan yerel kanıt deposuna aktaran
bir adli edinim aracıdır.

Kod veya YAML ile uğraşmadan kullanmak için macOS'ta `IMAJER.app`, Windows'ta
`IMAJER.exe` dosyasına çift tıklayın. Arayüz Safari, Chrome veya normal bir
tarayıcı sekmesi açmadan IMAJER'in kendi penceresinde çalışır. Apple Silicon,
Intel Mac, Windows x64 ve Windows ARM64 paketleri `dist/packages/` altında
üretilir. Masaüstü kurulum rehberi:
[`MASAUSTU_KULLANIM.md`](MASAUSTU_KULLANIM.md). Kısa arayüz rehberi:
[`ARAYUZ_KULLANIMI.md`](ARAYUZ_KULLANIMI.md).

Adım adım kurulum, job örnekleri, SSH/WinRM kullanımı, resume, verify ve
troubleshooting için [`KULLANIM_KILAVUZU.md`](KULLANIM_KILAVUZU.md) dosyasına
bakın.

Ekip arkadaşları için ekran görüntülü kısa anlatım:
[`EKIP_HIZLI_KULLANIM.md`](EKIP_HIZLI_KULLANIM.md).
Paylaşılabilir PDF sürümü:
[`EKIP_HIZLI_KULLANIM.pdf`](EKIP_HIZLI_KULLANIM.pdf).
Node.js ile Chrome, Chromium veya Edge bulunan geliştirme ortamında PDF şu
komutla yeniden üretilir:

```sh
node docs/ekip-kullanim/build-pdf.mjs
```

> Bu araç yalnız açık yasal yetkiyle kullanılmalıdır. Canlı edinim hedef
> belleğini, işletim sistemi loglarını ve filesystem metadata'sını değiştirir.
> “Zero disk footprint” yalnız hedefte RAM/disk imajı veya staging parçası
> oluşturulmaması anlamındadır.

## Bileşenler

- `imajer`: Windows ve macOS üzerinde çalışan controller.
- `imajer-agent`: Hedefte salt-okunur kaynak erişimi ve framed streaming.
- 8 MiB doğrulanmış chunk'lar, 2 GiB parçalı RAW çıktı.
- Disk için güvenli ofset resume; RAM kesilirse yeni sıfır-ofset attempt.
- Chunk/session/tam dosya SHA-256, sıralı Merkle root.
- JSONL timeline, JSON/PDF rapor ve Ed25519 imzalı evidence index.
- Linux RAM için imzalı AVML 0.20 ve hedef içinde loopback-only TCP streaming.

## Derleme

Go 1.26.5 gereklidir.

```sh
make test
make vet
make reproducible
make cross VERSION=0.6.8
make desktop-packages VERSION=0.6.8
```

Üretilen controller hedefleri:

- `darwin/amd64`, `darwin/arm64`
- `windows/amd64`, `windows/arm64`

Agent hedefleri:

- `darwin/amd64`, `darwin/arm64`
- `linux/amd64`, `linux/arm64`
- `windows/amd64`, `windows/arm64`

Darwin agent'ları ve Windows ARM64 agent'ı aynı makinedeki yerel edinimlerde
kullanılır. Uzak hedef desteği Linux amd64/arm64 ve Windows amd64 ile
sınırlıdır.

CGO kullanılmaz.

Derleme çıktıları `dist/` altına yazılır ve kaynak kontrolüne eklenmez. Resmî
release paketi yayımlandığında sağlanan `SHA256SUMS` dosyasıyla binary
bütünlüğü ayrıca doğrulanmalıdır.

`desktop-packages` hedefi şu son kullanıcı paketlerini üretir:

- `IMAJER-macOS-Apple-Silicon-<sürüm>.zip`
- `IMAJER-macOS-Intel-<sürüm>.zip`
- `IMAJER-Windows-x64-<sürüm>.zip`
- `IMAJER-Windows-ARM64-<sürüm>.zip`
- paketlerin SHA-256 değerlerini içeren `SHA256SUMS`

Tam `.app` ve Windows ZIP paketleme hedefi macOS üzerinde çalıştırılır.
macOS uygulaması ad-hoc imzalanır. Başka Mac'lerde uyarısız dağıtım için Apple
Developer ID ve notarization; Windows SmartScreen uyarısını kaldırmak için
Authenticode sertifikası gerekir. Agent binary'leri paket oluşturulurken ayrı
Ed25519 tool-release anahtarıyla imzalanan manifest üzerinden doğrulanır; özel
release anahtarı pakete dahil edilmez.

Yayınlanan masaüstü ZIP'leri son kullanıcı bilgisayarında Go, Node.js, Python
veya ayrı AVML kurulumu istemez. Desteklenen minimum istemci sistemleri macOS
12 Monterey, Windows 10 ve Windows Server 2016'dır. Windows masaüstü penceresi
işletim sistemiyle gelen Microsoft Edge'i, dosya seçiciler Windows PowerShell'i
kullanır. Bunlar kurumsal olarak kaldırılmışsa CLI kullanılabilir veya ilgili
Windows bileşenleri yeniden etkinleştirilmelidir.

GitHub'daki `desktop-packages` workflow'u aynı dört paketi manuel olarak veya
`v*` sürüm etiketi gönderildiğinde otomatik üretir. Etiket çalışmasında ZIP'ler
ve `SHA256SUMS` dosyası GitHub Release olarak yayımlanır. Çalıştırılmadan önce
PKCS#8 Ed25519 özel anahtarı repository secret olarak
`TOOL_RELEASE_PRIVATE_PEM` adıyla tanımlanmalıdır. Workflow anahtarı loga veya
artifact'e eklemez.

Kaynak koddan yerel demo hazırlamak için:

```sh
make demo VERSION=0.6.8
```

## Anahtarlar ve imzalı araç paketi

Examiner ve tool-release anahtarları uygulama dışında oluşturulur:

```sh
openssl genpkey -algorithm ED25519 -out examiner-private.pem
chmod 600 examiner-private.pem
openssl pkey -in examiner-private.pem -pubout -out examiner-public.pem
```

Agent/araç bundle manifesti:

```sh
imajer tools sign \
  --spec example/tools.example.yaml \
  --key tool-release-private.pem \
  --out tool-manifest.json

imajer tools verify \
  --manifest tool-manifest.json \
  --key tool-release-public.pem
```

Masaüstü paketi, Linux `amd64` ve `arm64` için Microsoft AVML `0.20.0`
binary'lerini resmî GitHub release varlıklarından yalnızca paketleme
sırasında indirir. Yayınlanan SHA-256 değerleri eşleşmezse paketleme durur;
doğrulanan dosyalar Ed25519 imzalı tool manifestine eklenir. Edinim sırasında
internetten indirme yapılmaz. RAM verisi, AVML ile agent arasında yalnızca
hedefin `127.0.0.1` arayüzüne bağlı TCP akışından geçirilir; hedef diskte RAM
imajı oluşturulmaz. Büyük delillerin yerel hash ve rapor üretimi sırasında SSH
oturumu zaman aşımına uğrasa bile cleanup öncesinde yeni bir doğrulanmış bağlantı
kurulur. FTK gibi proprietary binary'ler dağıtıma dahil
değildir. İmzalı araç manifesti bu dosyaların envanterini ve hedefteki
hash doğrulamasını destekler; ancak standart FTK CLI'nin dosya-destination
tabanlı edinim akışı strict zero-image-footprint modunda otomatik seçilmez.
Windows disk ediniminde salt-okunur native reader kullanılır. Vendor tarafından
desteklenen stdout/range-streaming adaptörü ayrıca sağlanırsa imzalı harici
provider olarak `provider: external` ile eklenebilir. Adaptörün stdout'u yalnız
RAW baytları üretmeli ve şu argümanları kabul etmelidir: `--source`,
`--offset`, `--size`, `--sector-size`. Adaptör de signed-tool manifestiyle
yüklenip uzak hash'i doğrulanır.

## Kullanım

Örnek job: [`example/job.example.yaml`](example/job.example.yaml).

```sh
imajer wizard --out job.yaml
imajer discover --job job.yaml
imajer acquire --job job.yaml
imajer resume --job job.yaml
imajer acquire --job job.yaml \
  --profile disk --disk-path /dev/sdb --disk-id SERIAL \
  --disk-model MODEL --disk-size 1000204886016 --disk-sector-size 512
imajer verify \
  --case-dir /evidence/CASE-2026-001/EVID-001 \
  --public-key /offline-trust/examiner-public.pem
```

`acquire` ve `resume` için profil, çıktı, imzalama anahtarı, hedef
host/port/user, parola ortam değişkeni ve disk/provider alanları flag ile
override edilebilir. Eksik disk alanları flag'lerle tamamlanacaksa YAML,
override uygulanmadan önce değil sonrasında doğrulanır.

SSH kullanımı doğrulanmış `known_hosts` ister. WinRM yalnız HTTPS kabul eder;
CA PEM verilmesi önerilir. Parola job içine yazılmaz; `password_env` yalnız
ortam değişkeninin adını içerir. Uygun bir SSH agent/key veya parola ortam
değişkeni yoksa TTY üzerinde echo kapalı etkileşimli parola istenir.

`verify` komutundaki harici `--public-key`, kanıt paketindeki public key'in
değiştirilmesine karşı bağımsız güven kökü sağlar. Verilmezse paket içindeki
public key ve manifestteki key ID birlikte doğrulanır.

## Edinim sağlayıcıları

Linux RAM seçimi AVML, ardından tam kernel eşleşmeli LiME'dir. Linux disk için
`dc3dd`, `dd`, ardından native read-only erişim kullanılır. Windows disk
`\\.\PhysicalDriveN` üzerinden salt-okunur açılır. Windows RAM agent'ı
vendor-signed WinPmem sürücüsünü yükler, akışı doğrudan controller'a yollar ve
işlem sonunda sürücüyü kaldırmayı dener.

Linux `amd64` ve `arm64` masaüstü paketlerinde AVML 0.20 hazırdır. LiME,
AVML'nin hedef kernel güvenlik politikası nedeniyle kullanılamadığı ortamlarda
yalnız hedef çekirdekle birebir uyumlu imzalı `.ko` sağlanırsa kullanılabilir.

## Doğrulama durumları

- `verified_continuous`: Tek kesintisiz oturum; uzak stream, yerel stream ve
  bağımsız yerel SHA-256 aynıdır.
- `chunk_verified_composite`: Disk ofset-resume edilmiştir. Her chunk ve oturum
  doğrulanmıştır fakat imaj farklı zamanlardan gelen disk parçaları içerir.
- `incomplete`: Genellikle kesilmiş RAM attempt'i.
- `failed`: Resume state korunmuş kontrollü başarısızlık.

Resume edilmiş canlı diskte bağımsız bir “uzak kaynak tam hash'i” iddia edilmez.
Bu sınır PDF ve JSON manifestte açıkça kaydedilir.

## Hedef footprint ve temizlik

Agent/araç yüklemeleri rastgele `imajer-<nonce>` dizininde yapılır. Controller,
yerelde kanıt paketinde saklanan case marker'ın uzak SHA-256 değerini
doğrulamadan hiçbir geçici uzak yolu temizlemez. Bağlantı kaybında
temizlenemeyen dosya, driver veya modül raporda residual footprint olarak
görünür.

Temizlik, iptal edilmiş edinim context'inden bağımsız fakat varsayılan iki
dakikalık sınırlı bir context ile yürür (`retry.cleanup_timeout`). Böylece
cleanup denenir ancak süresiz takılı kalmaz.

Kaynak cihaz hiçbir zaman yazma modunda açılmaz; VSS/LVM snapshot veya uzak
staging oluşturulmaz.

## Genel `exit status 1` mesajı

`exit status 1` yalnız alt sürecin başarısız kapandığını belirtir. Kök neden,
bu satırdan önceki stderr/canlı kayıt ile vaka dizinindeki `events.jsonl`,
artifact `sessions.jsonl` ve `state.json` kayıtlarında bulunur. Ağ kesintisinde
disk aynı job ve output ile doğrulanmış ofsetten devam eder. RAM devam etmez;
her yeniden çalışma yeni bir sıfır-ofset attempt üretir. Nihai karar için:

```sh
imajer verify --case-dir /evidence/CASE-ID/EVIDENCE-ID
```

çıktısında `ACQUISITION_VERIFIED` ve `PACKAGE_INTEGRITY_OK` aranmalıdır.

## Geliştirme doğrulaması

`PLAN_EKSIK_ANALIZI.md`, plan maddelerinin güncel kapanma durumunu ve yalnız
laboratuvar ortamında doğrulanabilecek kalan kabul işlerini içerir. Yerel
regular-file edinimi controller ve gerçek agent binary'leriyle; bağımsız
yeniden hash, canonical signed evidence index ve `verify` komutu dahil uçtan
uca test edilmiştir.

İzole Linux hedefinde gerçek SSH/SFTP testi:

```sh
./test/remote-ssh/run-test.sh
```

Kurulum için [`test/remote-ssh/README.md`](test/remote-ssh/README.md), son
doğrulanmış sonuç için
[`test/remote-ssh/TEST_SONUCU.md`](test/remote-ssh/TEST_SONUCU.md) dosyasına
bakın. Linux arm64 kesintisiz SSH regular-file akışı doğrulanmıştır. WinRM,
raw disk, kernel modülü, bağlantı-kopması ve Windows driver kullanımı yine de
yetkili izole VM/hardware laboratuvarında ayrıca doğrulanmalıdır.
