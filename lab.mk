# lab.mk — lab targets that sit alongside the kubebuilder-generated Makefile.
# Add "include lab.mk" near the top of Makefile after scaffolding.

TARGET_ADDR ?= 127.0.0.1:9339
CERT_DIR    ?= certs
CLAB_TOPO   ?= lab/topology.clab.yml

##@ Lab

.PHONY: certs
certs: ## Generate a self-signed CA plus server/client certs for gNMI TLS.
	@mkdir -p $(CERT_DIR)
	@openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
		-keyout $(CERT_DIR)/ca.key -out $(CERT_DIR)/ca.crt \
		-subj "/CN=gnmi-lab-ca" 2>/dev/null
	@openssl req -newkey rsa:2048 -nodes \
		-keyout $(CERT_DIR)/server.key -out $(CERT_DIR)/server.csr \
		-subj "/CN=target.lab" 2>/dev/null
	@openssl x509 -req -in $(CERT_DIR)/server.csr -days 3650 \
		-CA $(CERT_DIR)/ca.crt -CAkey $(CERT_DIR)/ca.key -CAcreateserial \
		-extfile <(printf "subjectAltName=DNS:target.lab,IP:127.0.0.1") \
		-out $(CERT_DIR)/server.crt 2>/dev/null
	@echo "certs written to $(CERT_DIR)/"

.PHONY: target-up
target-up: certs ## Run the gnxi in-memory gNMI target in the foreground.
	gnmi_target \
		-bind_address $(TARGET_ADDR) \
		-config lab/target-config.json \
		-key $(CERT_DIR)/server.key \
		-cert $(CERT_DIR)/server.crt \
		-ca $(CERT_DIR)/ca.crt \
		-alsologtostderr

.PHONY: probe
probe: ## Phase 1: Capabilities -> Get -> Set -> Get against $(TARGET_ADDR).
	go run ./cmd/gnmiprobe -target $(TARGET_ADDR) -ca $(CERT_DIR)/ca.crt

.PHONY: lab-up
lab-up: ## Deploy the containerlab topology (SR Linux etc).
	sudo containerlab deploy -t $(CLAB_TOPO)

.PHONY: lab-down
lab-down: ## Tear down the containerlab topology.
	sudo containerlab destroy -t $(CLAB_TOPO) --cleanup

.PHONY: lab-inspect
lab-inspect: ## Show running lab nodes and their addresses.
	sudo containerlab inspect -t $(CLAB_TOPO)

.PHONY: kind-of-real
kind-of-real: ## Print where the operator, cluster and targets actually live.
	@echo "node:      $$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo '<no cluster>')"
	@echo "target:    $(TARGET_ADDR)"
	@echo "clab net:  $$(docker network inspect clab -f '{{range .IPAM.Config}}{{.Subnet}}{{end}}' 2>/dev/null || echo '<not deployed>')"
