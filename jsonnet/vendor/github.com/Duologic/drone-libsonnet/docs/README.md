# package drone

Jsonnet library for generating Drone CI configuration file.

## Install

```
jb install github.com/Duologic/drone-libsonnet@master
```

## Usage

```jsonnet
// drone.jsonnet
local drone = import 'github.com/Duologic/drone-libsonnet/main.libsonnet';

local pipeline = drone.pipeline.docker;
local step = drone.pipeline.docker.step;

local pipelines = {
  build_pipeline:
    pipeline.new('build pipeline')
    + pipeline.withSteps([
      step.new('build', image='golang:1.19')
      + step.withCommands(['go build ./...']),
      step.new('test', image='golang:1.19')
      + step.withCommands(['go test ./...']),
    ]),
};

drone.render.getDroneObjects(pipelines)

```

Render the YAML file:

```console
drone jsonnet --stream \
              --format \
              --source <(jsonnet -J vendor/ drone.jsonnet) \
              --target .drone.yml
```

> Originally the intention was to render YAML with `std.manifestYamlStream()`,
> however at Grafana Labs we noticed that this function suffers from
> performance issues (taking 16 seconds to render a 23K LoC YAML). Its much
> faster to render the drone pipeline into json with
> `drone.render.getDroneObjects()` and use the `drone` cli tooling to do the
> YAML conversion. Alternatively `jsonnet -y` can be used, which delivers
> a valid YAML stream (json as valid YAML) but it might not look as nice.

```

```

## Subpackages

* [pipeline](drone/pipeline.md)
* [secret](drone/secret.md)
* [signature](drone/signature.md)
* [template](drone/template.md)

