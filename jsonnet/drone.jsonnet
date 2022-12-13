local drone = import 'github.com/Duologic/drone-libsonnet/main.libsonnet';

local pipeline = drone.pipeline.docker;
local step = drone.pipeline.docker.step;

local image_to_push = 'grafana/wait-for-github';

local images = {
  alpine: 'alpine:3.17.0',
  drone_plugin: 'plugins/docker',
  drone_cli: 'drone/cli:latest',
};

local modified_paths = [
  'go.mod',
  'go.sum',
  '**/*.go',
];

local pipelines = {
  build_test:
    pipeline.new('build pipeline')
    + pipeline.withSteps([
      step.new('build + test', image=images.drone_plugin)
      + step.withSettings({
        dry_run: true,
        password: {
          from_secret: 'docker-hub-password',
        },
        repo: image_to_push,
        tags: 'latest',
        username: {
          from_secret: 'docker-hub-username',
        },
      }),
    ])
    + pipeline.trigger.onModifiedPaths(modified_paths)
    + pipeline.trigger.onPullRequest(),
  build_test_push:
    pipeline.new('build and push pipeline')
    + pipeline.withSteps([
      step.new('build + test + push', image=images.drone_plugin)
      + step.withSettings({
        dry_run: false,
        password: {
          from_secret: 'docker-hub-password',
        },
        repo: image_to_push,
        tags: 'latest',
        username: {
          from_secret: 'docker-hub-username',
        },
      }),
    ])
    + pipeline.trigger.onModifiedPaths(modified_paths)
    + pipeline.trigger.onPushToMainBranch(),
} + {
  lint_pipeline:
    pipeline.new('linters')
    + pipeline.withSteps([
      // Disabled. We're getting "unable to start container process: exec:
      // "/bin/sh": stat /bin/sh: no such file or directory: unknown". The
      // drone-cli image doesn't have a shell. We need to find a replacement.
      //   step.new('check .drone.yml drift', image=images.drone_cli)
      //   + step.withCommands([
      //     'make check-drone-yml-drift',
      //   ]),
      step.new('lint jsonnet', image=images.alpine)
      + step.withCommands([
        'apk add --no-cache make jsonnet',
        'make lint-jsonnet',
      ]),
    ])
    + pipeline.trigger.onPullRequest(),
};

local secrets = {
  secrets: [
    drone.secret.new('docker-hub-username', 'secret/data/common/docker-hub', 'username'),
    drone.secret.new('docker-hub-password', 'secret/data/common/docker-hub', 'password'),
  ],
};

drone.render.getDroneObjects(pipelines + secrets)
