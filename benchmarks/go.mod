module github.com/blairtcg/protego/benchmarks

go 1.22.0

require (
	github.com/afex/hystrix-go v0.0.0-20180502004556-fa1af6a1f4f5
	github.com/cep21/circuit/v4 v4.1.0
	github.com/rubyist/circuitbreaker v2.2.1+incompatible
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/streadway/handy v0.0.0-20200128134331-0f66f006fb2e
	protego v0.0.0
)

require (
	github.com/cenk/backoff v2.2.1+incompatible // indirect
	github.com/facebookgo/clock v0.0.0-20150410010913-600d898af40a // indirect
	github.com/peterbourgon/g2s v0.0.0-20170223122336-d4e7ad98afea // indirect
	github.com/smartystreets/goconvey v1.8.1 // indirect
)

replace protego => ../
