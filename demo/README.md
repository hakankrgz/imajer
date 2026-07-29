# Yerel demo

Bu klasörde private key, sentetik RAW kaynak ve kanıt çıktısı kaynak kontrolüne
eklenmez. Demo dosyalarını yerel olarak üretmek için proje kökünde:

```sh
make demo VERSION=0.6.6
```

Ardından:

```sh
./dist/imajer ui
```

veya macOS'ta `IMAJER-ARAYUZ.command` dosyasına çift tıklayın. Demo yalnız
regular-file tabanlı fonksiyon testi içindir; gerçek fiziksel edinim değildir.
