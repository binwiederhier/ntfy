module heckel.io/ntfy

go 1.18

require (
	cloud.google.com/go/firestore v1.13.0 // indirect
	cloud.google.com/go/storage v1.33.0 // indirect
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/emersion/go-smtp v0.18.0
	github.com/gabriel-vasile/mimetype v1.4.2
	github.com/gorilla/websocket v1.5.0
	github.com/mattn/go-sqlite3 v1.14.17
	github.com/olebedev/when v1.0.0
	github.com/stretchr/testify v1.8.1
	github.com/urfave/cli/v2 v2.25.7
	golang.org/x/crypto v0.14.0
	golang.org/x/oauth2 v0.12.0 // indirect
	golang.org/x/sync v0.3.0
	golang.org/x/term v0.13.0
	golang.org/x/time v0.3.0
	google.golang.org/api v0.143.0
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/emersion/go-smtp => github.com/emersion/go-smtp v0.17.0 // Pin version due to breaking changes, see #839

require github.com/pkg/errors v0.9.1 // indirect

require (
	firebase.google.com/go/v4 v4.12.1
	github.com/SherClockHolmes/webpush-go v1.2.0
	github.com/prometheus/client_golang v1.17.0
	github.com/stripe/stripe-go/v74 v74.30.0
)

require (
	cloud.google.com/go v0.110.8 // indirect
	cloud.google.com/go/compute v1.23.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.2 // indirect
	cloud.google.com/go/longrunning v0.5.1 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emersion/go-sasl v0.0.0-20220912192320-0145f2c60ead // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.3.1 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.1 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.4.1-0.20230718164431-9a2bf3000d16 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/appengine/v2 v2.0.5 // indirect
	google.golang.org/genproto v0.0.0-20230920204549-e6e6cdab5c13 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230920204549-e6e6cdab5c13 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230920204549-e6e6cdab5c13 // indirect
	google.golang.org/grpc v1.58.2 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
