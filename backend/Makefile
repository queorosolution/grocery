# Used by "sam build" (BuildMethod: makefile). One target per function, one go.mod for all.
# ARTIFACTS_DIR is set by SAM.
define build_fn
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -tags lambda.norpc -o $(ARTIFACTS_DIR)/bootstrap ./cmd/$(1)
endef

.PHONY: build-ApiFunction build-WorkerFunction build-TimerFunction build-SyncFunction

build-ApiFunction:
	$(call build_fn,api)

build-WorkerFunction:
	$(call build_fn,worker)

build-TimerFunction:
	$(call build_fn,timer)

build-SyncFunction:
	$(call build_fn,sync)
