# IMAJER

[![CI](https://github.com/hakankrgz/imajer/actions/workflows/ci.yml/badge.svg)](https://github.com/hakankrgz/imajer/actions/workflows/ci.yml)
[![Lisans: Apache-2.0](https://img.shields.io/badge/Lisans-Apache--2.0-blue.svg)](LICENSE)

IMAJER, Linux ve Windows sistemlerden fiziksel disk veya canlı RAM verisini
yerel kanıt deposuna aktaran bir adli edinim aracıdır. Hedefte imaj ya da
staging dosyası oluşturmadan doğrulanabilir RAW çıktı, rapor ve imzalı kanıt
indeksi üretir.

> [!WARNING]
> Bu araç yalnız açık yasal yetkiyle kullanılmalıdır. Canlı edinim hedef
> belleğini, sistem loglarını ve dosya sistemi metadata'sını değiştirebilir.
> “Zero disk footprint”, yalnız hedefte RAM/disk imajı veya staging parçası
> oluşturulmaması anlamındadır.

> [!IMPORTANT]
> Proje geliştirme ve laboratuvar aşamasındadır. Gerçek vaka veya üretim
> kullanımı öncesinde kuruma özgü kabul testleri ve chain-of-custody
> prosedürleri uygulanmalıdır. Güncel doğrulama durumu
> [`PLAN_EKSIK_ANALIZI.md`](docs/PLAN_EKSIK_ANALIZI.md) içinde açıklanır.

## Temel özellikler

- macOS ve Windows için masaüstü arayüzü
- Yerel, SSH ve WinRM üzerinden disk/RAM edinimi
- 8 MiB doğrulanmış chunk'lar ve 2 GiB parçalı RAW çıktı
- Disk için doğrulanmış ofsetten devam etme
- SHA-256, Merkle root ve Ed25519 imzalı kanıt indeksi
- JSONL zaman çizelgesi ile JSON ve PDF raporları
- İmzalı agent ve araç manifesti doğrulaması

## Hızlı başlangıç

Son kullanıcı paketini [GitHub Releases](https://github.com/hakankrgz/imajer/releases)
sayfasından indirin ve işletim sisteminize uygun uygulamayı açın:

- macOS: `IMAJER.app`
- Windows: `IMAJER.exe`

Masaüstü kurulumu için
[`MASAUSTU_KULLANIM.md`](docs/MASAUSTU_KULLANIM.md), arayüz adımları için
[`ARAYUZ_KULLANIMI.md`](docs/ARAYUZ_KULLANIMI.md) dosyasını izleyin.

CLI ile temel akış:

```sh
imajer wizard --out job.yaml
imajer discover --job job.yaml
imajer acquire --job job.yaml
imajer verify \
  --case-dir /evidence/CASE-ID/EVIDENCE-ID \
  --public-key /offline-trust/examiner-public.pem
```

Örnek yapılandırma: [`example/job.example.yaml`](example/job.example.yaml).

## Kaynaktan derleme

Go 1.26.5 gereklidir.

```sh
make test
make vet
make cross VERSION=0.6.8
make desktop-packages VERSION=0.6.8
```

Derleme çıktıları `dist/` altında oluşturulur. Yayın paketlerini kullanırken
birlikte verilen `SHA256SUMS` dosyasını doğrulayın.

## Platform desteği

| Bileşen | Desteklenen hedefler |
| --- | --- |
| Controller | macOS amd64/arm64, Windows amd64/arm64 |
| Yerel agent | macOS amd64/arm64, Windows amd64/arm64 |
| Uzak agent | Linux amd64/arm64, Windows amd64 |

Minimum istemci sistemleri macOS 12 Monterey, Windows 10 ve Windows Server
2016'dır. Sağlayıcılar ve platform sınırları ayrıntılı kullanım kılavuzunda
açıklanır.

## Dokümantasyon

- [Tüm belgeler](docs/README.md)
- [Tam kullanım kılavuzu](docs/KULLANIM_KILAVUZU.md)
- [Masaüstü kurulumu](docs/MASAUSTU_KULLANIM.md)
- [Arayüz kullanımı](docs/ARAYUZ_KULLANIMI.md)
- [Ekran görüntülü hızlı rehber](docs/EKIP_HIZLI_KULLANIM.md)
- [Paylaşılabilir PDF rehberi](docs/EKIP_HIZLI_KULLANIM.pdf)
- [Doğrulama ve eksik analizi](docs/PLAN_EKSIK_ANALIZI.md)
- [SSH uçtan uca test sonucu](test/remote-ssh/TEST_SONUCU.md)

## Katkı, güvenlik ve lisans

Katkı yapmak için [`CONTRIBUTING.md`](CONTRIBUTING.md) dosyasına bakın.
Güvenlik açıklarını veya hassas adli verileri herkese açık issue olarak
paylaşmayın; [`SECURITY.md`](SECURITY.md) içindeki özel bildirim yolunu
kullanın.

Proje [Apache License 2.0](LICENSE) ile lisanslanır. Dağıtılan üçüncü taraf
bileşenlerin lisansları ilgili paketleme dizinlerinde ayrıca korunur.
