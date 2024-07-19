module heckel.io/ntfy/v2

go 1.21

toolchain go1.21.3

require (
	cloud.google.com/go/firestore v1.15.0 // indirect
	cloud.google.com/go/storage v1.43.0 // indirect
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/emersion/go-smtp v0.18.0
	github.com/gabriel-vasile/mimetype v1.4.4
	github.com/gorilla/websocket v1.5.3
	github.com/mattn/go-sqlite3 v1.14.22
	github.com/olebedev/when v1.0.0
	github.com/stretchr/testify v1.9.0
	github.com/urfave/cli/v2 v2.27.2
	golang.org/x/crypto v0.25.0
	golang.org/x/oauth2 v0.21.0 // indirect
	golang.org/x/sync v0.7.0
	golang.org/x/term v0.22.0
	golang.org/x/time v0.5.0
	google.golang.org/api v0.188.0
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/emersion/go-smtp => github.com/emersion/go-smtp v0.17.0 // Pin version due to breaking changes, see #839

require github.com/pkg/errors v0.9.1 // indirect

require (
	firebase.google.com/go/v4 v4.14.1
	github.com/SherClockHolmes/webpush-go v1.3.0
	github.com/microcosm-cc/bluemonday v1.0.27
	github.com/prometheus/client_golang v1.19.1
	github.com/stripe/stripe-go/v74 v74.30.0
)

require (
	cloud.google.com/go v0.115.0 // indirect
	cloud.google.com/go/auth v0.7.1 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.3 // indirect
	cloud.google.com/go/compute/metadata v0.5.0 // indirect
	cloud.google.com/go/iam v1.1.11 // indirect
	cloud.google.com/go/longrunning v0.5.10 // indirect
	github.com/AlekSi/pointer v1.2.0 // indirect
	github.com/MicahParks/keyfunc v1.9.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/emersion/go-sasl v0.0.0-20231106173351-e73c9f7bad43 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt v3.2.2+incompatible // indirect
	github.com/golang-jwt/jwt/v4 v4.5.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.5 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.55.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.53.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.53.0 // indirect
	go.opentelemetry.io/otel v1.28.0 // indirect
	go.opentelemetry.io/otel/metric v1.28.0 // indirect
	go.opentelemetry.io/otel/trace v1.28.0 // indirect
	golang.org/x/net v0.27.0 // indirect
	golang.org/x/sys v0.22.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/appengine/v2 v2.0.6 // indirect
	google.golang.org/genproto v0.0.0-20240711142825-46eb208f015d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240711142825-46eb208f015d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240711142825-46eb208f015d // indirect
	google.golang.org/grpc v1.65.0 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
