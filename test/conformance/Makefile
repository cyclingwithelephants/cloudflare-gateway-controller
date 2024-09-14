.PHONY: build-test-runner
build-test-runner:
	docker build -t conformance-test-runner:latest . &&\
	kind load docker-image --name conformance conformance-test-runner:latest

.PHONY: build-app
build-app:
	docker build -t cloudflare-gateway-controller:latest -f ../../Dockerfile ../.. &&\
	kind load docker-image --name conformance cloudflare-gateway-controller:latest

.PHONY: create-kind-cluster
create-kind-cluster:
	kind create cluster --name conformance
	#kind export kubeconfig --name conformance --kubeconfig $(KIND_KUBE_CONFIG)

deploy-app:
	kustomize build ../../deploy/for-testing | kubectl apply -f -

re-deploy-app: build-app deploy-app

deploy-secret:
	kubectl apply -f ../../secrets/secret.yaml -n cloudflare-gateway-controller-system

deploy-gateway-class:
	kubectl apply -f gateway-class.yaml

#delete-app:
#	kustomize build ../../config/manager | kubectl delete -f -
#	kustomize build ../../config/crd | kubectl delete -f -
#	kustomize build ../../config/default | kubectl delete -f -/
#	kustomize build ../../config/network-policy | kubectl delete -f -
#	kustomize build ../../config/rbac | kubectl delete -f -

.PHONY: run-conformance-tests
run-conformance-tests:
	kubectl delete pod conformance --ignore-not-found
	kubectl apply -f conformance-rbac.yaml
	kubectl run -i conformance \
		--image=conformance-test-runner:latest --image-pull-policy=Never \
		--overrides='{ "spec": { "serviceAccountName": "conformance" }  }' \
		--restart=Never -- go test -v ./conformance -args \
			--gateway-class=conformance-test \
			--debug \
			--supported-features="Gateway,HTTPRoute"

.PHONY: everything
everything: create-kind-cluster build-app build-test-runner deploy-app deploy-secret deploy-gateway-class run-conformance-tests

cleanup:
	kind delete cluster --name conformance
