module github.com/simplesurance/goordinator

go 1.16

require (
	github.com/cenkalti/backoff v2.2.1+incompatible
	github.com/golang/mock v1.5.0
	github.com/google/go-github/v35 v35.0.0
	github.com/itchyny/gojq v0.12.2
	github.com/pelletier/go-toml v1.9.0
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	github.com/sykesm/zap-logfmt v0.0.4
	github.com/thecodeteam/goodbye v0.0.0-20170927022442-a83968bda2d3
	go.uber.org/atomic v1.8.0
	go.uber.org/goleak v1.1.10
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.18.1
	golang.org/x/lint v0.0.0-20210508222113-6edffad5e616 // indirect
	golang.org/x/oauth2 v0.0.0-20210514164344-f6687ab2804c
	golang.org/x/tools v0.1.4 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace github.com/google/go-github/v35 => github.com/simplesurance/go-github/v35 v35.3.1-0.20210616142130-cb65de31f15d
