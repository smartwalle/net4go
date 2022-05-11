module github.com/smartwalle/net4go/examples

require (
	github.com/gorilla/websocket v1.4.2
	github.com/smartwalle/net4go v0.0.49
	github.com/smartwalle/queue v0.0.2
	go.uber.org/ratelimit v0.2.0
)

require github.com/smartwalle/net4go/ws v0.0.20 // indirect

replace (
	github.com/smartwalle/net4go => ../
	//github.com/smartwalle/net4go/quic => ../quic
	github.com/smartwalle/net4go/ws => ../ws
)

go 1.18
