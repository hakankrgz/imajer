# Gerçek SSH E2E Test Sonucu

Tarih: 2026-07-28 22:58 UTC
IMAJER sürümü: `0.6.2`
Sonuç: **BAŞARILI**

## Test kapsamı

Docker içindeki Ubuntu 24.04 hedefi gerçek TCP, SSH ve SFTP üzerinden
kullanıldı. Controller macOS arm64 üzerinde, agent Linux arm64 üzerinde native
çalıştı. Root olmayan `forensic` hesabı parolasız `sudo` ile yükseltildi. Test
fiziksel disk yerine hedefteki salt-okunur 24 MiB sentetik RAW dosyasını
kullandı.

Bu test, dışarıdaki ayrı bir fiziksel sunucu testi değildir. Ağ protokolü ve
uzak Linux süreç sınırı gerçektir; bağlantı loopback üzerindedir.

## Doğrulanan akış

1. Ed25519 SSH anahtarıyla giriş
2. Zorunlu `known_hosts` host-key kontrolü ve değişmiş anahtarın reddi
3. Signed-tool manifest ve agent SHA-256 doğrulaması
4. Agent'ın SFTP ile hedef `/dev/shm` alanına geçici yüklenmesi
5. Linux/arm64/passwordless-sudo hedef keşfi, doğru disk kimliğinin kabulü ve
   yanlış disk kimliğinin reddi
6. Üç adet 8 MiB chunk ile doğrudan in-memory streaming
7. Yerelde 16 MiB `disk.001` ve 8 MiB `disk.002` üretimi
8. Uzak stream, yerel anlık ve bağımsız yeniden okuma hash doğrulaması
9. Canonical evidence index ve detached Ed25519 imza doğrulaması
10. Uzak geçici agent ve staging kalıntısı kontrolü; test container'ının
    otomatik kaldırılması
11. Kanıt paketinde private key/credential kalıbı taraması

## Sonuç değerleri

- Hedef: Ubuntu 24.04, Linux `6.12.76-linuxkit`, arm64, passwordless sudo
- Kaynak boyutu: `25.165.824` byte
- Chunk boyutu: `8.388.608` byte
- Session: `1`
- Retry: `0`
- Durum: `verified_continuous`
- Provider: `native-readonly`
- Merkle root:
  `202a14062836826e3edc9466904d108c80d85175ab87189215166abb7a2aaeb2`
- Uzak ve yerel mantıksal SHA-256:
  `95aeaae03b56c171cf88753c821630a3c24f1fcf406cec3e17d56781aa3f8369`
- Paket doğrulaması:
  `PACKAGE_INTEGRITY_OK: signed evidence index valid; verified=1 partial=0`
- Uzak `/dev/shm` ve `/tmp` kalıntısı: yok
- Hedef `/evidence` içeriği: yalnız salt-okunur `source.raw`
- Secret taraması: `SECRET_SCAN_OK`

## Tekrar çalıştırma

Proje kökünde:

```sh
./test/remote-ssh/run-test.sh
```

Son başarılı çalıştırmanın dosyaları:

```text
test/remote-ssh/runtime/latest/
```

Test container'ı varsayılan olarak otomatik kaldırılır. Açık tutmak için:

```sh
IMAJER_TEST_KEEP_CONTAINER=1 ./test/remote-ssh/run-test.sh
```

## Kalan gerçek-sistem testleri

- Gerçek ağ cihazında TCP/SSH bağlantısını chunk ortasında kesip `resume`
  doğrulaması (otomatik fault transport testi mevcut)
- Linux amd64 hedef
- Gerçek salt-okunur raw block device
- AVML ve hedef-kernel LiME RAM edinimi
- Windows Server WinRM HTTPS ve WinPmem
- NTFS ve exFAT yerel kanıt diskleri
