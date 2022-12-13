local crdsonnet = import 'github.com/Duologic/crdsonnet/crdsonnet/main.libsonnet';
local d = import 'github.com/jsonnet-libs/docsonnet/doc-util/main.libsonnet';

local render = import './render.libsonnet';
local schema = import './schema.libsonnet';

local parsed =
  crdsonnet.fromSchema(
    'drone',
    schema,
    render='dynamic',
  ).drone
;


local package(name, parents=[]) = {
  '#': d.package.new(
         name,
         'github.com/Duologic/drone-libsonnet',
         '',
         'main.libsonnet',
       )
       + d.package.withUsageTemplate(|||
         local drone = import "%s";

         drone.%s%s.<attribute>
       ||| % [
         'github.com/Duologic/drone-libsonnet',
         std.join('.', parents) + (if std.length(parents) != 0 then '.' else ''),
         name,
       ]),
};

local getName(n) =
  std.splitLimit(n, '_', 1)[1];

local lib =
  {
    '#': d.package.new(
           'drone',
           'github.com/Duologic/drone-libsonnet',
           'Jsonnet library for generating Drone CI configuration file.',
           'main.libsonnet',
         )
         + d.package.withUsageTemplate(|||
           %s
           ```

           Render the YAML file:

           ```console
           drone jsonnet --stream \
                         --format \
                         --source <(jsonnet -J vendor/ drone.jsonnet) \
                         --target .drone.yaml
           ```

           > Originally the intention was to render YAML with `std.manifestYamlStream()`,
           > however at Grafana Labs we noticed that this function suffers from
           > performance issues (taking 16 seconds to render a 23K LoC YAML). Its much
           > faster to render the drone pipeline into json with
           > `drone.render.getDroneObjects()` and use the `drone` cli tooling to do the
           > YAML conversion. Alternatively `jsonnet -y` can be used, which delivers
           > a valid YAML stream (json as valid YAML) but it might not look as nice.

           ```
         ||| % (importstr 'example/drone.jsonnet')),
  }
  + {
    // Strip `kind_` from name
    [getName(k)]:
      parsed[k]
      + package(getName(k))
    for k in std.objectFields(parsed)
    if std.startsWith(k, 'kind_')
       // `kind_pipeline` is covered by `pipeline.<type>`
       && k != 'kind_pipeline'
  }
  + {
    // Strip `pipeline_` from name and nest in `pipeline` object
    pipeline:
      {
        [getName(k)]:
          parsed[k]
          + package(getName(k), ['pipeline'])
        for k in std.objectFields(parsed)
        if std.startsWith(k, 'pipeline_')
      }
      + package('pipeline'),
  }
  + {
    secret+: {
      '#new':: d.fn(
        '`new` is a shorthand for creating a new secret object',
        args=[
          d.arg('name', d.T.string),
          d.arg('path', d.T.string),
          d.arg('key', d.T.string),
        ]
      ),
      new(name, path, key):
        self.withKind()
        + self.withName(name)
        + self.get.withPath(path)
        + self.get.withName(key),
    },

    local pipeline = super.pipeline,
    pipeline: {
      [k]:
        pipeline[k]
        {
          '#new':: d.fn(
            '`new` is a shorthand for creating a new pipeline object',
            args=[d.arg('name', d.T.string)]
          ),
          new(name):
            self.withKind()
            + self.withType()
            + self.withName(name),

          '#withParallelStepsMixin':: d.fn(
            |||
              Pipeline steps are executed sequentially by default. You can optionally
              describe your build steps as a directed acyclic graph with `depends_on`.

              '`withParallelStepsMixin` will configure each step with `<dependsOn>` to
              ensure these steps are executed in parallel. By default it will set
              `depends_on` to 'clone'.
            |||,
            args=[
              d.arg('steps', d.T.array),
              d.arg('dependsOn', d.T.array, ['clone']),
            ]
          ),
          withParallelStepsMixin(steps, dependsOn=['clone']): {
            steps+: [
              step + pipeline[k].steps.withDependsOn(dependsOn)
              for step in steps
            ],
          },

          steps:: {},
          step:  // Use singular instead of plural
            super.steps
            + package('step', ['pipeline'])
            + {
              '#dependsOnCloneStep':: d.fn(
                |||
                  `dependsOnCloneStep` is a shorthand for `withDependsOn(['clone'])
                |||,
              ),
              dependsOnCloneStep():
                self.withDependsOn('clone'),

              // Extend when with useful shortcuts
              when+: {
                '#onPushToBranch':: d.fn(
                  |||
                    `onPushToBranch` will conditionally limit this step to a `push` event
                    on `<branch_name>`
                  |||,
                  args=[d.arg('branch_name', d.T.string)]
                ),
                onPushToBranch(branch_name):
                  self.event.withIncludeMixin(['push'])
                  + self.branch.withIncludeMixin([branch_name]),

                '#onPushToMasterBranch':: d.fn(
                  |||
                    `onPushToMasterBranch` will conditionally limit this step to a `push`
                    event on `master` branch
                  |||,
                ),
                onPushToMasterBranch(): self.onPushToBranch('master'),

                '#onPushToMainBranch':: d.fn(
                  |||
                    `onPushToMainBranch` will conditionally limit this step to a `push`
                    event on `main` branch
                  |||,
                ),
                onPushToMainBranch(): self.onPushToBranch('main'),

                '#onPullRequest':: d.fn(
                  |||
                    `onPullRequest` will conditionally limit this step to
                    a `pull_request` event
                  |||,
                ),
                onPullRequest(): self.event.withIncludeMixin(['pull_request']),

                '#onSuccess':: d.fn(
                  |||
                    `onSuccess` will conditionally limit this step to a successful
                    pipeline
                  |||,
                ),
                onSuccess(): self.withStatusMixin(['success']),

                '#onFailure':: d.fn(
                  |||
                    `onFailure` will conditionally limit this step to a pipeline failure.
                  |||,
                ),
                onFailure(): self.withStatusMixin(['failure']),
              },
            }
            + (if k == 'docker'
               then {
                 '#new':: d.fn(
                   '`new` is a shorthand for creating a new step object',
                   args=[
                     d.arg('name', d.T.string),
                     d.arg('image', d.T.string),
                   ]
                 ),
                 new(name, image):
                   self.withName(name)
                   + self.withImage(image),

                 withPrivileged(): super.withPrivileged(true),
               }
               else {
                 '#new':: d.fn(
                   '`new` is a shorthand for creating a new step object',
                   args=[d.arg('name', d.T.string)]
                 ),
                 new(name):
                   self.withName(name),
               }),

          clone+: {
            '#withDisable':: d.fn(
              |||
                `withDisable` is a shorthand for disabling cloning, it will also unset
                `clone.depth` and `clone.retries` to avoid confusion
              |||
            ),
            withDisable(): {
              clone: {
                disable: true,
                // hide other attributes on disable
                depth:: 0,
                retries:: 0,
              },
            },
          },

          // Extend trigger with useful shortcuts
          trigger+: {
            '#onPushToBranches':: d.fn(
              |||
                `onPushToBranches` will conditionally limit pipeline execution to
                a `push` event on `<branches>`
              |||
            ),
            onPushToBranches(branches):
              self.event.withIncludeMixin('push')
              + self.branch.withIncludeMixin(branches),

            '#onPullRequestAndPushToBranches':: d.fn(
              |||
                `onPullRequestAndPushToBranches` will conditionally limit pipeline
                execution to `push` and `pull_request` events on `<branches>`
              |||
            ),
            onPullRequestAndPushToBranches(branches):
              self.event.withIncludeMixin(['pull_request', 'push'])
              + self.branch.withIncludeMixin(branches),

            '#onPushToMasterBranch':: d.fn(
              |||
                `onPushToMasterBranch` will conditionally limit pipeline
                execution to a `push` event on `master` branch
              |||
            ),
            onPushToMasterBranch():
              self.onPushToBranches(['master']),

            '#onPushToMainBranch':: d.fn(
              |||
                `onPushToMainBranch` will conditionally limit pipeline
                execution to a `push` event on `main` branch
              |||
            ),
            onPushToMainBranch():
              self.onPushToBranches(['main']),

            '#onPullRequest':: d.fn(
              |||
                `onPullRequest` will conditionally limit pipeline
                execution to a `pull_request` event
              |||
            ),
            onPullRequest():
              self.event.withIncludeMixin('pull_request'),

            '#onPromotion':: d.fn(
              |||
                `onPromotion` will conditionally limit pipeline
                execution to a `promotion` event
              |||
            ),
            onPromotion(targets):
              self.event.withIncludeMixin('promote')
              + self.target.withIncludeMixin(targets),

            '#onCronSchedule':: d.fn(
              |||
                `onCronSchedule` will conditionally limit pipeline
                execution to a `cron` event with `<schedule>`
              |||
            ),
            onCronSchedule(schedule):
              self.event.withIncludeMixin('cron')
              + self.cron.withIncludeMixin(schedule),

            '#hourly':: d.fn(
              |||
                `hourly` will conditionally limit pipeline
                execution to a `hourly` `cron` event.
              |||
            ),
            hourly(): self.onCronSchedule('hourly'),

            '#nightly':: d.fn(
              |||
                `hourly` will conditionally limit pipeline
                execution to a `nightly` `cron` event.
              |||
            ),
            nightly(): self.onCronSchedule('nightly'),

            '#onModifiedPaths':: d.fn(
              |||
                `onModifiedPaths` will conditionally limit pipeline execution on changes
                to these paths (requires plugin)
              |||
            ),
            onModifiedPaths(paths):
              self.paths.withIncludeMixin(paths),

            '#onModifiedPath':: d.fn(
              '`onModifiedPath` shorthand for `onModifiedPaths([path])'
            ),
            onModifiedPath(path):
              self.onModifiedPaths([path]),
          },
        }
        + (if 'services' in pipeline[k]
           then {
             services:: {},
             service:  // Use singular instead of plural
               super.services
               + package('service', ['pipeline']),
           }
           else {})
      for k in std.objectFields(pipeline)
    },

    render: render,
  };

lib
