# IMAJER Kullanım Kılavuzu

Bu kılavuz `imajer 0.6.5` geliştirme sürümünün kurulmasını, vaka
yapılandırmasını, uzak hedefin keşfedilmesini, RAM/disk edinimini, kesinti
sonrası devam etmeyi ve kanıt paketini doğrulamayı anlatır.

> **Yasal ve operasyonel uyarı:** Bu yazılım yalnız açık yasal yetkiyle ve
> kurum prosedürlerine uygun olarak kullanılmalıdır. Canlı edinim hedef RAM'ini,
> filesystem metadata'sını ve SSH/WinRM/Windows Event Log gibi denetim
> kayıtlarını değiştirebilir. “Zero disk footprint” yalnız hedefte RAM veya disk
> imajı/staging parçası oluşturulmaması anlamına gelir.

## 1. Hızlı başlangıç

Tipik kullanım sırası:

```text
Anahtarları hazırla
        ↓
Agent ve araç manifestini imzala
        ↓
Job YAML oluştur
        ↓
imajer discover
        ↓
Disk kimliğini operatör olarak doğrula
        ↓
imajer acquire
        ↓
Gerekirse imajer resume
        ↓
imajer verify
```

Temel komutlar:

```sh
./dist/imajer version
./dist/imajer discover --job job.yaml
./dist/imajer acquire --job job.yaml
./dist/imajer resume --job job.yaml
./dist/imajer verify --case-dir /kanit/CASE-ID/EVIDENCE-ID \
  --public-key /guvenli-konum/examiner-public.pem
```

### 1.1. Masaüstü uygulamasında basit kullanım

1. `IMAJER.app` veya Windows'ta `IMAJER.exe` dosyasına çift tıklayın.
2. **Yeni işlem oluştur** sekmesini açın ve vaka bilgilerini doldurun.
3. **Linux / SSH** seçin; Raspberry Pi IP adresini ve kullanıcı adını yazın.
4. Anahtar, kanıt klasörü veya başka bir yerel yol gerektiğinde **Gözat…** ya
   da **Klasör seç…** düğmesini kullanın. Uzak `/dev/mmcblk0` gibi disk
   yollarını elle yazmayın.
5. **Bağlan ve diskleri getir** düğmesine basın. İlk bağlantıda gösterilen SSH
   fingerprint'i Raspberry Pi üzerinde `ssh-keygen -lf
   /etc/ssh/ssh_host_ed25519_key.pub` çıktısıyla karşılaştırın.
6. Model, boyut ve cihaz yolunu kontrol ederek fiziksel diski listeden açıkça
   seçin. `[BAĞLI/SİSTEM]` uyarısı, diskin canlı olarak değişebileceğini
   belirtir.
7. Önce **Bilgileri kontrol et**, sonra **İmajı başlat** düğmesine basın.
8. İşlem bittiğinde sağ taraftaki **Bütünlük karşılaştırmaları** alanında uzak
   ve yerel SHA-256 değerlerinin **EŞLEŞİYOR** olduğunu kontrol edin.
9. Son olarak **Kanıt doğrula** sekmesinde vaka klasörünü seçip **Kanıtı
   doğrula** düğmesine basın. `İMAJ DOĞRULANDI` ve `İMZA GEÇERLİ` rozetlerinin
   ikisi de görünmelidir.

### 1.2. Raspberry Pi Ubuntu sunum hazırlığı

Raspberry Pi üzerinde Ubuntu ARM64, SSH ve `lsblk` bulunmalıdır. Kullanacağınız
hesap root değilse IMAJER'in etkileşimsiz yeniden bağlanabilmesi için sunum
süresince parolasız sudo gerekir:

```sh
sudo apt update
sudo apt install -y openssh-server sudo util-linux
sudo systemctl enable --now ssh
echo 'KULLANICI_ADI ALL=(root) NOPASSWD: ALL' | \
  sudo tee /etc/sudoers.d/imajer-demo
sudo chmod 0440 /etc/sudoers.d/imajer-demo
sudo visudo -cf /etc/sudoers.d/imajer-demo
sudo -n id -u
uname -m
hostname -I
lsblk -b -o NAME,PATH,MODEL,SERIAL,WWN,SIZE,LOG-SEC,TYPE,MOUNTPOINT
```

`sudo -n id -u` çıktısı `0`, `uname -m` çıktısı `aarch64` olmalıdır. Sunum
sonunda geçici yetkiyi kaldırın:

```sh
sudo rm /etc/sudoers.d/imajer-demo
```

ARM64 disk edinimi native agent ile desteklenir. ARM64 RAM edinimi yalnız
Raspberry Pi'nin çalışan çekirdeğiyle birebir uyumlu, imzalı LiME modülü
hazırlanmışsa kullanılmalıdır; sınıf sunumunda böyle bir modül yoksa **Tüm
disk** profilini seçin.

## 2. Desteklenen binary'ler

`dist/` dizininde aşağıdaki geliştirme binary'leri bulunur:

| Dosya | Kullanıldığı sistem |
|---|---|
| `imajer-darwin-arm64` | Apple Silicon macOS controller |
| `imajer-darwin-amd64` | Intel macOS controller |
| `imajer-windows-amd64.exe` | Windows x64 controller |
| `imajer-windows-arm64.exe` | Windows ARM64 controller |
| `imajer-agent-darwin-amd64` | Intel macOS yerel edinim yardımcısı |
| `imajer-agent-darwin-arm64` | Apple Silicon yerel edinim yardımcısı |
| `imajer-agent-linux-amd64` | Linux x64 hedef |
| `imajer-agent-linux-arm64` | Linux ARM64 hedef |
| `imajer-agent-windows-amd64.exe` | Windows Server x64 hedef |
| `imajer-agent-windows-arm64.exe` | Windows ARM64 yerel edinim yardımcısı |

Mevcut macOS mimarisine uygun binary'yi kolay kullanım için kopyalamak veya
yeniden adlandırmak mümkündür. Hazır dosyaların bütünlüğü:

```sh
cd dist
shasum -a 256 -c SHA256SUMS
cd ..
```

Windows PowerShell karşılığı:

```powershell
Get-FileHash .\dist\imajer-windows-amd64.exe -Algorithm SHA256
Get-Content .\dist\SHA256SUMS
```

## 3. Kaynaktan derleme

Gereksinim: Go 1.26.5.

Yerel build:

```sh
make test
make vet
make build VERSION=0.6.5
```

Tüm hedefler:

```sh
make reproducible VERSION=0.6.5
make cross VERSION=0.6.5
```

Derlemelerde CGO kapalıdır. Ürünler `dist/` altına yazılır.

## 4. İki farklı anahtarın amacı

Sistem iki ayrı Ed25519 güven alanı kullanır:

1. **Examiner anahtarı:** Son kanıt indeksini imzalar.
2. **Tool-release anahtarı:** Uzak hedefe yüklenecek agent ve acquisition
   araçlarının manifestini imzalar.

Bu anahtarları birbirinden ayırmak önerilir. Private key'ler kanıt çıktı
dizininin dışında tutulmalıdır.

### 4.1. Examiner anahtarı

```sh
openssl genpkey -algorithm ED25519 -out examiner-private.pem
chmod 600 examiner-private.pem
openssl pkey -in examiner-private.pem -pubout -out examiner-public.pem
```

`examiner-private.pem` PKCS#8 formatında olmalıdır. Unix sistemlerinde private
key izni `0600` veya daha sıkı değilse uygulama işlemi reddeder.

Bağımsız doğrulama yapacak kişiye yalnız `examiner-public.pem` verilir.

### 4.2. Tool-release anahtarı

```sh
openssl genpkey -algorithm ED25519 -out tool-release-private.pem
chmod 600 tool-release-private.pem
openssl pkey -in tool-release-private.pem \
  -pubout -out tool-release-public.pem
```

## 5. İmzalı agent/tool paketi hazırlama

Uzak SSH veya WinRM ediniminde controller'ın yükleyeceği agent, imzalı tool
manifestinde bulunmalıdır. AVML, LiME veya harici adapter yüklenecekse bunlar da
aynı kurala tabidir.

Resmî masaüstü paketlerinde Microsoft AVML `0.20.0` binary'leri Linux
`amd64` ve `arm64` için hazır gelir. Paketleme sırasında resmî release SHA-256
değerleri doğrulanır, ardından AVML uygulamanın Ed25519 imzalı tool manifestine
eklenir. Linux hedef tarandıktan sonra uygun AVML dosyası arayüz tarafından
otomatik seçilir; edinim sırasında internet bağlantısı gerekmez. RAM çıktısı
AVML'den agent'a hedefin yalnızca `127.0.0.1` arayüzünde dinleyen TCP soketi
üzerinden aktarılır ve hedef diskte RAM imajı oluşturulmaz. Uzun yerel hash ve
rapor işlemleri SSH oturumunu boşta bırakırsa cleanup yeni bir doğrulanmış
bağlantıyla yürütülür.

### 5.1. Bundle dizini oluşturma

Örnek Linux x64 paketi:

```text
tool-bundle/
├── imajer-agent-linux-amd64
├── avml
└── tools.yaml
```

`tools.yaml`:

```yaml
- name: imajer-agent
  version: "0.6.5"
  os: linux
  arch: amd64
  path: ./imajer-agent-linux-amd64
  license: Apache-2.0

- name: avml
  version: "operator-pinned-version"
  os: linux
  arch: amd64
  path: ./avml
  license: MIT
```

LiME için hedef çekirdek tam olarak belirtilmelidir:

```yaml
- name: lime
  version: "operator-built"
  os: linux
  arch: arm64
  kernel: "6.8.0-31-generic"
  path: ./lime.ko
  license: GPL-2.0
```

Manifesti, dosya yollarının doğru çözüldüğü bundle dizininde oluşturun:

```sh
cd tool-bundle
../dist/imajer tools sign \
  --spec tools.yaml \
  --key /guvenli-konum/tool-release-private.pem \
  --out tool-manifest.json
```

Manifesti ve bundle dosyalarını doğrulayın:

```sh
../dist/imajer tools verify \
  --manifest tool-manifest.json \
  --key /guvenli-konum/tool-release-public.pem
```

Başarılı çıktı örneği:

```text
VERIFIED: signed manifest and 2 artifact(s)
```

> Uygulama acquisition sırasında internetten agent veya araç indirmez.
> Kullanılacak tüm üçüncü taraf binary'lerinin lisansı, kaynağı ve güvenilirliği
> operatör tarafından önceden doğrulanmalıdır.

## 6. Hedef sistem hazırlığı

### 6.1. Linux/SSH

Gereksinimler:

- SSH erişimi.
- Doğrulanmış `known_hosts` dosyası.
- Root oturumu veya `sudo -n` ile parolasız yetki yükseltme.
- Agent yüklemek için geçici dizinde çalışma izni.
- Fiziksel disk/RAM kaynağını okumaya yetecek izin.

Host anahtarını kör biçimde kabul etmeyin. Fingerprint'i kurumun güvenilir
başka bir kanalından doğruladıktan sonra `known_hosts` dosyasına ekleyin.

Örnek:

```sh
ssh-keyscan -H server.example > known_hosts.candidate
ssh-keygen -lf known_hosts.candidate
```

Gösterilen fingerprint bağımsız kaynaktan doğrulandıktan sonra dosyayı vaka
için korunan `known_hosts` dosyası olarak kullanın.

### 6.2. Windows/WinRM

Gereksinimler:

- Windows Server x64 hedef.
- WinRM'in önceden etkinleştirilmiş olması.
- Yalnız HTTPS listener.
- Administrator yetkisi.
- Güvenilen CA PEM dosyası veya sistem trust store tarafından doğrulanan
  sunucu sertifikası.
- Basic, NTLM veya Kerberos authentication.

Uygulama WinRM'i hedefte etkinleştirmez ve HTTP bağlantısını kabul etmez.
Yalnız certificate fingerprint vermek desteklenmez; özel CA kullanılıyorsa
`ca_file` verilmelidir.

### 6.3. Yerel laboratuvar modu

`transport: local`, regular-file tabanlı güvenli fonksiyon testi için
kullanılabilir. Bu mod fiziksel uzak edinim yerine controller-agent protokolünü
aynı bilgisayarda test eder.

## 7. Job YAML oluşturma

Etkileşimli taslak:

```sh
./dist/imajer wizard --out job.yaml
```

Wizard bir başlangıç dosyası üretir. Özellikle disk ID, model, boyut ve sektör
boyutu hedef discovery sonucuyla operatör tarafından karşılaştırılmalıdır.

### 7.1. Ortak alanlar

```yaml
case:
  id: CASE-2026-001
  evidence_id: EVID-001
  examiner: Ada Lovelace
  organization: Example DFIR
  authority_ref: IR-2026-42
  notes: Yetkili canlı edinim
  authorized: true

acquisition:
  profile: disk
  chunk_size: 8388608
  segment_size: 2147483648

output:
  directory: /absolute/path/evidence
  signing_key: /guvenli-konum/examiner-private.pem

retry:
  max_attempts: 10
  connect_timeout: 30s
  chunk_timeout: 5m
  cleanup_timeout: 2m
```

Kurallar:

- `case.id` ve `evidence_id`: yalnız harf, rakam, `.`, `_`, `-`.
- `authorized`: mutlaka `true`.
- `profile`: `ram`, `disk` veya `both`.
- `chunk_size`: 1–64 MiB; varsayılan 8 MiB.
- `segment_size`: chunk boyutunun tam katı, en fazla 4 GiB; varsayılan 2 GiB.
- `signing_key`: mutlak yol ve evidence output ağacının dışında.
- `cleanup_timeout`: 1 saniye–30 dakika.

### 7.2. Linux SSH disk edinimi

```yaml
case:
  id: CASE-2026-001
  evidence_id: EVID-DISK-001
  examiner: Ada Lovelace
  organization: Example DFIR
  authority_ref: IR-2026-42
  authorized: true

target:
  transport: ssh
  host: server.example
  port: 22
  user: forensic
  private_key: /absolute/path/id_ed25519
  known_hosts: /absolute/path/known_hosts

acquisition:
  profile: disk
  chunk_size: 8388608
  segment_size: 2147483648
  disk:
    path: /dev/sda
    id: ata-SERIAL-NUMBER
    model: "MODEL NAME"
    size: 1000204886016
    sector_size: 512
    provider: auto

output:
  directory: /evidence
  signing_key: /offline-keys/examiner-private.pem

agent:
  local_path: /tool-bundle/imajer-agent-linux-amd64
  tool_manifest: /tool-bundle/tool-manifest.json
  trust_public_key: /offline-keys/tool-release-public.pem

retry:
  max_attempts: 10
  connect_timeout: 30s
  chunk_timeout: 5m
  cleanup_timeout: 2m
```

Linux disk provider sırası:

1. `dc3dd`
2. `dd`
3. Agent'ın native read-only okuyucusu

Strict signed-tool politikası isteniyorsa hedef PATH'teki `dd`/`dc3dd` yerine
`provider: native` kullanılması önerilir.

### 7.3. Linux SSH RAM edinimi

AVML örneği:

```yaml
acquisition:
  profile: ram
  ram:
    id: volatile-memory
    provider: avml
    tool_name: avml
    tool_local_path: /tool-bundle/avml

agent:
  local_path: /tool-bundle/imajer-agent-linux-amd64
  tool_manifest: /tool-bundle/tool-manifest.json
  trust_public_key: /offline-keys/tool-release-public.pem
```

LiME örneği:

```yaml
acquisition:
  profile: ram
  ram:
    id: volatile-memory
    provider: lime
    tool_name: lime
    tool_local_path: /tool-bundle/lime.ko
```

LiME modülü hedefin tam kernel sürümü ve mimarisi için önceden güvenilir
ortamda derlenmiş olmalıdır. Linux ARM64 RAM edinimi için LiME gereklidir.

### 7.4. Windows WinRM disk edinimi

```yaml
case:
  id: CASE-2026-002
  evidence_id: EVID-DISK-001
  examiner: Ada Lovelace
  organization: Example DFIR
  authority_ref: IR-2026-43
  authorized: true

target:
  transport: winrm
  host: windows-server.example
  port: 5986
  user: FORENSIC-LAB\examiner
  auth: ntlm
  password_env: IMAJER_TARGET_PASSWORD
  ca_file: C:\Secure\winrm-ca.pem

acquisition:
  profile: disk
  disk:
    path: \\.\PhysicalDrive0
    id: DISK-SERIAL-NUMBER
    model: "DISK MODEL"
    size: 1000204886016
    sector_size: 512
    provider: native

output:
  directory: D:\Evidence
  signing_key: C:\OfflineKeys\examiner-private.pem

agent:
  local_path: C:\ToolBundle\imajer-agent-windows-amd64.exe
  tool_manifest: C:\ToolBundle\tool-manifest.json
  trust_public_key: C:\OfflineKeys\tool-release-public.pem

retry:
  max_attempts: 10
  connect_timeout: 30s
  chunk_timeout: 5m
  cleanup_timeout: 2m
```

Windows disk için önerilen provider `native` değeridir. Kaynak
`\\.\PhysicalDriveN` salt-okunur açılır.

Parolayı YAML içine yazmayın. `password_env`, yalnız ortam değişkeninin adıdır.
Değer yoksa desteklenen interaktif terminalde uygulama parolayı echo kapalı
olarak sorabilir.

### 7.5. Windows WinRM RAM edinimi

```yaml
acquisition:
  profile: ram
  ram:
    id: volatile-memory
    provider: winpmem
```

Agent vendor-signed WinPmem sürücüsünü yükler, veriyi doğrudan stream eder ve
işlem sonunda exact service/driver'ı kaldırmayı dener. Yükleme ve işletim
sistemi audit etkileri rapor footprint'inin parçasıdır.

### 7.6. RAM ve disk birlikte

```yaml
acquisition:
  profile: both
  ram:
    id: volatile-memory
    provider: avml
    tool_name: avml
    tool_local_path: /tool-bundle/avml
  disk:
    path: /dev/sda
    id: ata-SERIAL-NUMBER
    model: "MODEL NAME"
    size: 1000204886016
    sector_size: 512
    provider: native
```

`both` profilinde RAM her zaman önce alınır.

### 7.7. Güvenli yerel fonksiyon testi

Uzak sunucuya bağlanmadan controller-agent ve kanıt paketini test etmek için
küçük bir regular file oluşturun:

```sh
mkdir -p local-lab
dd if=/dev/urandom of=local-lab/source.raw bs=1m count=2
```

Examiner anahtarını Bölüm 4.1'deki gibi oluşturduktan sonra mutlak yollarla:

```yaml
case:
  id: CASE-LOCAL-001
  evidence_id: EVID-LOCAL-001
  examiner: Lab Examiner
  organization: Local Lab
  authority_ref: FUNCTIONAL-TEST
  authorized: true

target:
  transport: local

acquisition:
  profile: disk
  chunk_size: 1048576
  segment_size: 2097152
  disk:
    path: /absolute/path/local-lab/source.raw
    id: local-synthetic-source
    model: synthetic-regular-file
    size: 2097152
    sector_size: 512
    provider: native

output:
  directory: /absolute/path/local-lab/evidence
  signing_key: /absolute/path/examiner-private.pem

agent:
  local_path: /absolute/path/dist/imajer-agent

retry:
  max_attempts: 3
  connect_timeout: 5s
  chunk_timeout: 30s
  cleanup_timeout: 30s
```

Çalıştırın:

```sh
./dist/imajer acquire --job local-job.yaml
./dist/imajer verify \
  --case-dir /absolute/path/local-lab/evidence/CASE-LOCAL-001/EVID-LOCAL-001 \
  --public-key /absolute/path/examiner-public.pem
```

Son olarak kaynak ve segment hash'ini karşılaştırabilirsiniz:

```sh
shasum -a 256 local-lab/source.raw
shasum -a 256 \
  local-lab/evidence/CASE-LOCAL-001/EVID-LOCAL-001/artifacts/disk/disk.001
```

Bu örnek yalnız protokol/bütünlük fonksiyon testidir; fiziksel disk veya RAM
edinim kabul testinin yerine geçmez.

## 8. Credential kullanımı

Credential veya private-key içeriğini YAML, log veya rapora yazmayın.

Seçenekler:

- SSH agent.
- Şifresiz veya passphrase korumalı SSH private key.
- `password_env` ile süreç ortam değişkeni.
- TTY üzerinde interaktif gizli parola/passphrase girişi.

Ortam değişkenini iş bittikten sonra temizleyin:

```sh
unset IMAJER_TARGET_PASSWORD
```

PowerShell:

```powershell
Remove-Item Env:IMAJER_TARGET_PASSWORD
```

## 9. Preflight ve hedef keşfi

Edinimden önce:

```sh
./dist/imajer discover --job job.yaml
```

Bu işlem:

- Agent'ı doğrular ve gerekiyorsa geçici olarak yükler.
- OS, mimari, hostname ve zamanı belirler.
- Administrator/root yetkisini kontrol eder.
- Kernel/build, RAM, disk envanteri ve araçları toplar.
- Seçili diskin kimliğini doğrular.
- Geçici footprint'i temizlemeyi dener.

Operatörün kontrol etmesi gereken alanlar:

- Hedef hostname.
- OS ve mimari.
- Admin/root durumu.
- Disk path.
- Seri/stable ID.
- Model.
- Toplam byte boyutu.
- Sektör boyutu.
- RAM provider uyarıları.

Fiziksel disk otomatik seçilmez. Yanlış disk seçimi edinim hatasına veya yanlış
kanıt kapsamına yol açabilir.

## 10. Edinimi başlatma

```sh
./dist/imajer acquire --job job.yaml
```

Ekranda şu bilgiler gösterilir:

- Artifact adı.
- Tamamlanan byte ve yüzde.
- Anlık ve ortalama hız.
- ETA.
- Retry sayısı.
- Güncel offset.
- Chunk doğrulama seviyesi.

Başarılı edinim sonunda controller:

1. Uzak ve yerel session hash'lerini karşılaştırır.
2. Logical RAW birleşimini bağımsız olarak yeniden okur ve hash'ler.
3. Chunk Merkle root üretir.
4. Artifact manifestini yazar.
5. JSON ve PDF raporu oluşturur.
6. Evidence index'i examiner private key ile imzalar.
7. Hedefteki geçici footprint'i temizlemeyi dener.

## 11. Flag ile job alanlarını değiştirme

`acquire` ve `resume` aşağıdaki override flag'lerini destekler:

```text
--profile
--output
--signing-key
--host
--port
--user
--password-env
--disk-path
--disk-id
--disk-model
--disk-size
--disk-sector-size
--disk-provider
--ram-provider
```

Örnek:

```sh
./dist/imajer acquire --job job.yaml \
  --output /Volumes/Evidence \
  --disk-path /dev/disk4 \
  --disk-id ata-EXACT-SERIAL \
  --disk-model "EXACT MODEL" \
  --disk-size 2000398934016 \
  --disk-sector-size 512 \
  --disk-provider native
```

Flag değerleri YAML üzerine uygulanır ve job son durumda tekrar doğrulanır.
Vaka/yetki ve transport güvenliği gibi temel alanlar yine YAML'da bulunmalıdır.

## 12. Kesinti sonrası devam

### 12.1. Disk

Aynı job ve aynı output diziniyle:

```sh
./dist/imajer resume --job job.yaml
```

Resume şartları:

- Aynı case ID ve evidence ID.
- Aynı artifact/source ID.
- Aynı disk path, model, boyut ve sektör boyutu.
- Aynı chunk ve segment boyutu.
- Önceki RAW segmentleri, chunk journal ve state değiştirilmemiş olmalı.

Controller en büyük kesintisiz doğrulanmış ofseti bulur. Disk kimliği değişmişse
resume reddedilir.

Resume edilmiş canlı disk sonucu:

```text
chunk_verified_composite
```

Bu sonuç tek bir atomik zamanı veya kesintisiz uzak tam-source hash'ini temsil
etmez.

### 12.2. RAM

RAM edinimi kaldığı yerden devam etmez. Kesilen attempt:

```text
memory-attempt-001
status: incomplete
```

olarak korunur. Yeni attempt sıfır ofsetten başlar. Yalnız kesintisiz tamamlanan
RAM attempt'i doğrulanmış kanıt sayılır.

## 13. Kanıt paketini doğrulama

Önerilen yöntem, paket dışından güvenilen examiner public key kullanmaktır:

```sh
./dist/imajer verify \
  --case-dir /evidence/CASE-2026-001/EVID-DISK-001 \
  --public-key /offline-trust/examiner-public.pem
```

Başarılı örnek:

```text
ACQUISITION_VERIFIED: disk (verified_continuous)
PACKAGE_INTEGRITY_OK: signed evidence index valid; verified=1 partial=0
```

Olası çıktılar:

- `ACQUISITION_VERIFIED`: Tamamlanmış artifact hash ve durum doğrulandı.
- `NON_EVIDENTIARY_PARTIAL`: Korunan fakat delil olarak tamamlanmamış attempt.
- `PACKAGE_INTEGRITY_OK`: İmza ve evidence tree doğrulandı.

`verify` şu durumlarda hata döndürür:

- RAW içinde tek byte değişiklik.
- Chunk hash veya Merkle uyumsuzluğu.
- Evidence index imza hatası.
- İndekste olmayan ek dosya.
- Eksik veya boyutu değişmiş dosya.
- Hiç tamamlanmış artifact bulunmaması.
- `failed` veya `running` artifact.

## 14. Raporu yeniden üretme

```sh
./dist/imajer report --job job.yaml
```

Bu komut mevcut state/session/probe kayıtlarından JSON ve PDF raporu yeniden
üretir ve evidence index'i tekrar imzalar. Aynı examiner signing key erişilebilir
olmalıdır.

Rapor yeniden üretildikten sonra tekrar doğrulayın:

```sh
./dist/imajer verify \
  --case-dir /evidence/CASE-ID/EVIDENCE-ID \
  --public-key /offline-trust/examiner-public.pem
```

## 15. Manuel cleanup

Acquisition ve discovery sonunda cleanup otomatik denenir. Ağ kesintisi
nedeniyle geçici agent/tool dosyaları kalmışsa:

```sh
./dist/imajer cleanup --job job.yaml
```

Cleanup:

- Yerelde saklanan case marker'ı okur.
- Case/evidence kimliğini kontrol eder.
- Uzak marker'ın SHA-256 değerini doğrular.
- Agent'ın driver/module cleanup işlemini çağırır.
- Yalnız marker'da kayıtlı tool/agent yollarını kaldırır.

Marker yoksa, hash uyuşmazsa veya path güvenli değilse hiçbir dosya silinmez.
Preinstalled agent marker'da `RemoveAgent: false` olarak tutulur ve silinmez.

`agent.keep_on_failure: true` ayarlanmışsa otomatik cleanup bilerek atlanır.
Bu seçenek yalnız laboratuvar incelemesi için kullanılmalı ve kalan footprint
raporda açıklanmalıdır.

## 16. Kanıt dizini yapısı

```text
OUTPUT/
└── CASE-ID/
    └── EVIDENCE-ID/
        ├── artifacts/
        │   ├── disk/
        │   │   ├── disk.001
        │   │   ├── disk.002
        │   │   ├── chunks.jsonl
        │   │   ├── sessions.jsonl
        │   │   ├── state.json
        │   │   └── artifact-manifest.json
        │   └── memory-attempt-001/
        │       ├── memory.001
        │       └── ...
        ├── events.jsonl
        ├── target-probe.json
        ├── case-report.json
        ├── case-report.pdf
        ├── evidence-index.json
        ├── evidence-index.sig
        └── examiner-public-key.pem
```

Kanıt paketi içindeki dosyaları acquisition sonrasında değiştirmeyin. Not veya
ek belge eklenmesi gerekiyorsa kurum prosedürüne göre yeni bir üst paket veya
yeni imzalı indeks üretin.

## 17. Doğrulama durumları

| Durum | Anlamı |
|---|---|
| `verified_continuous` | Tek kesintisiz session; uzak stream, yerel stream ve bağımsız yeniden okuma SHA-256 eşleşti |
| `chunk_verified_composite` | Resume edilmiş disk; her chunk/session doğrulandı fakat farklı zaman aralıklarının bileşimi |
| `incomplete` | Tamamlanmamış RAM veya kısmi attempt |
| `failed` | İş başarısız; doğrulanmış state korunmuş olabilir |

## 18. Sık karşılaşılan hatalar

### `case.authorized must be true`

Job içindeki açık yasal yetki onayı eksiktir:

```yaml
case:
  authorized: true
```

### `no SSH authentication method available`

Şunlardan en az biri sağlanmalıdır:

- Çalışan SSH agent.
- `private_key`.
- `password_env` değerinin süreç ortamında bulunması.
- İnteraktif TTY parola girişi.

### `known_hosts`

`target.known_hosts` eksik, okunamıyor veya host key eşleşmiyor. Host key'i kör
biçimde yenilemeyin; değişikliği bağımsız kanaldan doğrulayın.

### `preinstalled remote agent requires signed tool_manifest`

`agent.remote_path` verilmiş olsa bile preinstalled agent'ın SHA-256 değeri
signed manifestte bulunmalıdır:

```yaml
agent:
  remote_path: /opt/forensic/imajer-agent
  tool_manifest: /secure/tool-manifest.json
  trust_public_key: /secure/tool-release-public.pem
```

### `selected disk stable ID does not match`

Job'daki ID hedef discovery değerleriyle eşleşmiyor. Disk otomatik
değiştirilmez. Path, serial/by-id/PNP ID, model ve boyutu yeniden doğrulayın.

### `resume state does not match`

Case/source/layout alanlarından biri önceki acquisition state'inden farklıdır.
Önceki job'ın exact kopyasını kullanın; output, chunk/segment veya disk kimliğini
değiştirmeyin.

### `insufficient local evidence space`

Controller disk boyutu, olası RAM attempt'leri ve yüzde 5 rezerv için yeterli
alan bulamadı. Daha büyük bir evidence volume seçin.

### `signing key permissions are too broad`

Unix:

```sh
chmod 600 examiner-private.pem
```

Private key output evidence ağacının dışında olmalıdır.

### `evidence index is not in the required canonical JSON form`

`evidence-index.json` acquisition sonrasında yeniden biçimlendirilmiş veya
değiştirilmiştir. Orijinal paketi geri yükleyin. Dosyayı elle düzeltip başarı
iddia etmeyin.

### Cleanup başarısız

Hedef erişimi, marker ve credential durumunu kontrol edip:

```sh
./dist/imajer cleanup --job job.yaml
```

komutunu yeniden çalıştırın. Silinemeyen exact yolları ve driver/module
durumunu `events.jsonl` ile raporda muhafaza edin.

## 19. Önerilen operasyon kontrol listesi

Edinim öncesi:

- [ ] Yazılı yetki ve kapsam doğrulandı.
- [ ] Vaka ve evidence ID benzersiz.
- [ ] Controller saati/NTP durumu kaydedildi.
- [ ] Hedef hostname, OS ve mimari doğrulandı.
- [ ] Disk seri/model/boyut/sektör operatör tarafından seçildi.
- [ ] Examiner private key evidence dizini dışında.
- [ ] Tool manifest imzası doğrulandı.
- [ ] Agent/tool binary hash'leri doğrulandı.
- [ ] Yeterli yerel boş alan var.
- [ ] SSH host key veya WinRM CA güven zinciri doğrulandı.

Edinim sonrası:

- [ ] `ACQUISITION_VERIFIED` görüldü.
- [ ] `PACKAGE_INTEGRITY_OK` görüldü.
- [ ] Harici examiner public key kullanıldı.
- [ ] Artifact status kaydedildi.
- [ ] PDF ve JSON rapor incelendi.
- [ ] Residual footprint/cleanup uyarıları incelendi.
- [ ] Evidence volume write-protect/kurum prosedürüne alındı.
- [ ] Chain-of-custody kaydı kurum sisteminde tamamlandı.

## 20. Bilinen sınırlar

- Uzak SSH/WinRM, raw disk ve RAM provider'ları gerçek kurum laboratuvarında
  ayrıca kabul testinden geçirilmelidir.
- Resume edilmiş canlı disk farklı zamanlarda okunan chunk'lardan oluşur.
- Canlı fiziksel disk kesintisiz edinimde dahi tek atomik zamanı temsil etmez.
- FTK'ye özgü otomatik provider adapter'ı yoktur.
- Windows ARM64 controller vardır; uzak Windows agent yalnız amd64'tür.
- Linux ARM64 RAM yalnız exact-kernel güvenilir LiME modülüyle desteklenir.
- PDF Türkçe içeriklidir fakat mevcut font katmanı bazı Türkçe karakterleri
  ASCII karşılıklarına dönüştürür.

Güncel teknik eksik ve kabul durumu için
[`PLAN_EKSIK_ANALIZI.md`](PLAN_EKSIK_ANALIZI.md) dosyasına bakın.
