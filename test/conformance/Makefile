GATEWAY_CLASS = cloudflare-gateway-operator
SUPPORTED_FEATURES = HTTPRoute,GatewayClass
KIND_KUBE_CONFIG=$${HOME}/.kube/kind/config

.PHONY: build-test-runner
build-test-runner:
	docker build -t conformance-test-runner:latest . &&\
	kind load docker-image --name conformance conformance-test-runner:latest

.PHONY: build-app
build-app:
	docker build -t ghcr.io/cyclingwithelephants/cloudflare-gateway-controller:latest -f ../../Dockerfile ../.. &&\
	kind load docker-image --name conformance ghcr.io/cyclingwithelephants/cloudflare-gateway-controller:latest

.PHONY: create-kind-cluster
create-kind-cluster:
	kind create cluster --name conformance
	kind export kubeconfig --name conformance --kubeconfig $(KIND_KUBE_CONFIG)

deploy-app:
	kubectl apply -k ../../config/manager
	kubectl apply \
		-k ../../config/default/ \
		-k ../../config/network-policy \
		-k ../../config/rbac

delete-app:
	kubectl delete \
		-k ../../config/default/ \
		-k ../../config/network-policy \
		-k ../../config/rbac
	kubectl delete -k ../../config/manager

.PHONY: run-conformance-tests
run-conformance-tests: create-kind-cluster build-app build-test-runner deploy-app
	kubectl apply -f conformance-rbac.yaml
	kubectl run -i conformance \
		--image=conformance-test-runner:latest --image-pull-policy=Never \
		--overrides='{ "spec": { "serviceAccountName": "conformance" }  }' \
		--restart=Never -- go test -v ./conformance -args --gateway-class=$(GATEWAY_CLASS) --debug \
						        --supported-features=$(SUPPORTED_FEATURES)

cleanup:
	kind delete cluster --name conformance
