.PHONY: test
test:
	cd test && \
	jb install && \
	jsonnet -J vendor test.libsonnet

drone.json:
	curl https://json.schemastore.org/drone.json -O

docs:
	jsonnet -J vendor -S -c -m docs docs.jsonnet

