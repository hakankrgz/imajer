# IMAJER'a Katkı

Katkılar issue veya pull request üzerinden yapılabilir. Proje ayrıcalıklı
sistem erişimi ve adli kanıtla çalıştığı için örnekler ve hata kayıtları yalnız
sentetik veri içermelidir.

## Başlamadan önce

- Güvenlik açığını herkese açık issue'da paylaşmayın;
  [`SECURITY.md`](SECURITY.md) içindeki özel bildirim yolunu kullanın.
- Ham kanıt, müşteri/kurum adı, hedef IP/hostname, credential, private key,
  `known_hosts`, gerçek vaka numarası veya kişisel dosya yolu eklemeyin.
- Yeni davranış için test ekleyin; adli bütünlük iddialarını test veya açık
  dokümantasyonla destekleyin.
- Üçüncü taraf binary ya da büyük üretilmiş artefakt eklemeden önce lisansını,
  kaynağını ve doğrulama hash'ini belgeleyin.

## Yerel doğrulama

Go 1.26.5 ile:

```sh
go test ./...
go vet ./...
```

Daha kapsamlı CI eşdeğeri kontroller:

```sh
make test
make reproducible
make cross
```

İzole Docker hedefli SSH testi için
[`test/remote-ssh/README.md`](test/remote-ssh/README.md) dosyasını izleyin. Bu
test Docker ve yerel port kullanır; yalnız test amacıyla üretilen anahtar ve
runtime dosyaları kaynak kontrolüne eklenmemelidir.

## Pull request beklentileri

Pull request açıklamasında değişikliğin amacı, güvenlik/adli bütünlük etkisi,
çalıştırılan testler ve bilinen sınırlar yer almalıdır. Platforma özgü kodda
doğrulanan işletim sistemi ve mimariyi açıkça belirtin. Davranış değişiyorsa
README veya kullanım kılavuzunu aynı değişiklik içinde güncelleyin.

Katkı göndererek değişikliklerinizin projenin
[Apache License 2.0](LICENSE) koşullarıyla dağıtılmasını kabul etmiş olursunuz.
