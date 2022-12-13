local test = import 'github.com/jsonnet-libs/testonnet/main.libsonnet';

local drone = import 'drone-libsonnet/main.libsonnet';

test.new(std.thisFile)
+ test.case.new(
  'renderFromSingle',
  test.expect.eq(
    importstr 'out/.drone.single.yaml',
    drone.render.toYaml(
      drone.pipeline.docker.new('pipeline1'),
    ),
  )
)
+ test.case.new(
  'renderFromObjects',
  test.expect.eq(
    importstr 'out/.drone.object.yaml',
    drone.render.toYaml({
      pipeline1:
        drone.pipeline.docker.new('pipeline1'),
      pipeline2:
        drone.pipeline.docker.new('pipeline2'),
    }),
  )
)
+ test.case.new(
  'renderFromArray',
  test.expect.eq(
    importstr 'out/.drone.array.yaml',
    drone.render.toYaml([
      drone.pipeline.docker.new('pipeline1'),
      drone.pipeline.docker.new('pipeline2'),
    ]),
  )
)
+ test.case.new(
  'renderFromCombo',
  test.expect.eq(
    importstr 'out/.drone.combo.yaml',
    drone.render.toYaml({
      pipeline1:
        drone.pipeline.docker.new('pipeline1'),
      pipeline2:
        drone.pipeline.docker.new('pipeline2'),
      pipelines: [
        drone.pipeline.docker.new('pipeline3'),
        drone.pipeline.docker.new('pipeline4'),
      ],
    }),
  )
)
