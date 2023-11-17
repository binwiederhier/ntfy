module heckel.io/ntfy/v2

go 1.21

toolchain go1.21.3

require (
	cloud.google.com/go/firestore v1.14.0 // indirect
	cloud.google.com/go/storage v1.35.1 // indirect
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.3 // indirect
	github.com/emersion/go-smtp v0.18.0
	github.com/gabriel-vasile/mimetype v1.4.3
	github.com/gorilla/websocket v1.5.1
	github.com/mattn/go-sqlite3 v1.14.18
	github.com/olebedev/when v1.0.0
	github.com/stretchr/testify v1.8.4
	github.com/urfave/cli/v2 v2.25.7
	golang.org/x/crypto v0.15.0
	golang.org/x/oauth2 v0.14.0 // indirect
	golang.org/x/sync v0.5.0
	golang.org/x/term v0.14.0
	golang.org/x/time v0.4.0
	google.golang.org/api v0.151.0
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/emersion/go-smtp => github.com/emersion/go-smtp v0.17.0 // Pin version due to breaking changes, see #839

require github.com/pkg/errors v0.9.1 // indirect

require (
	firebase.google.com/go/v4 v4.12.1
	github.com/SherClockHolmes/webpush-go v1.3.0
	github.com/microcosm-cc/bluemonday v1.0.26
	github.com/prometheus/client_golang v1.17.0
	github.com/stripe/stripe-go/v74 v74.30.0
)

require (
	cloud.google.com/go v0.110.10 // indirect
	cloud.google.com/go/compute v1.23.3 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.5 // indirect
	cloud.google.com/go/longrunning v0.5.4 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emersion/go-sasl v0.0.0-20231106173351-e73c9f7bad43 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.4.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.0 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/net v0.18.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/xerrors v0.0.0-20231012003039-104605ab7028 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/appengine/v2 v2.0.5 // indirect
	google.golang.org/genproto v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20231106174013-bbf56f31fb17 // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
