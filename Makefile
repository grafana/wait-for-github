fmt-jsonnet:
	@echo "Formatting jsonnet files"
	@find . -name '*.libsonnet' -o -name '*.jsonnet' | xargs -n 1 jsonnetfmt -i

lint-jsonnet:
	@echo "Linting jsonnet files"
	@find . -name '*.libsonnet' -o -name '*.jsonnet' | xargs -I{} -n 1 sh -c 'jsonnetfmt -- "{}" | diff -u "{}" -'

# If we're running on CI (CI = true) then don't run in docker
ifeq ($(CI),true)
DRONE := drone
else
DRONE := docker run --pull always -e DRONE_SERVER -e DRONE_TOKEN --rm -v ${PWD}:${PWD} -w "${PWD}" drone/cli:latest
endif


.drone.yml: jsonnet/drone.jsonnet $(shell find jsonnet/vendor -name *.libsonnet)  ## Render .drone.yml pipeline file
	 $(DRONE) jsonnet --stream --format --jpath jsonnet/vendor --source jsonnet/drone.jsonnet --target .drone.yml
	 $(DRONE) lint --trusted .drone.yml
	 $(DRONE) sign --save grafana/wait-for-github

check-drone-yml-drift:
	@echo "Checking for drift in .drone.yml"
	# Ugly awk to drop the last 5 lines of the file (the signature)
	$(DRONE) jsonnet --stream --format --jpath jsonnet/vendor --source jsonnet/drone.jsonnet --stdout | bash -c 'diff -u <(awk -v n=5 "NR==FNR{total=NR;next} FNR==total-n+1{exit} 1" .drone.yml .drone.yml) -'
