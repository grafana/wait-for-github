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
