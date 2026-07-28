# Gerçek SSH uzak sunucu testi

Bu düzenek Docker içinde gerçek bir Linux SSH/SFTP hedefi oluşturur ve
IMAJER'in aşağıdaki zincirini uçtan uca çalıştırır:

1. Ed25519 SSH anahtarı ve doğrulanmış `known_hosts`
2. İmzalı Linux agent manifesti
3. SSH üzerinden `discover`
4. SFTP ile geçici agent yükleme ve uzak hash doğrulaması
5. 8 MiB chunk'larla doğrudan ağ akışı
6. İki parçalı yerel RAW çıktı
7. Uzak/yerel SHA-256 karşılaştırması
8. Kanıt indeksi ve Ed25519 imza doğrulaması
9. Uzak geçici agent kalıntısı kontrolü

Test fiziksel diske erişmez. Hedefte `/evidence/source.raw` adıyla oluşturulan
24 MiB sentetik ve salt-okunur dosya kullanılır. Container kök dosya sistemi
de salt-okunurdur; yalnız `/run`, `/tmp` ve `/dev/shm` geçici RAM alanıdır.

## Çalıştırma

Docker Desktop açıkken proje kökünden:

```sh
./test/remote-ssh/run-test.sh
```

Son çalıştırmanın job dosyası:

```text
test/remote-ssh/runtime/latest/job.yaml
```

Test sunucusunu durdurmak için:

```sh
docker rm -f imajer-remote-e2e
```

`runtime/` altındaki anahtarlar yalnız bu laboratuvar testi içindir ve Git
tarafından dışlanır. Gerçek sistemlerde bu anahtarları kullanmayın.
