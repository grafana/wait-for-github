# package step



## Install

```
jb install github.com/Duologic/drone-libsonnet@master
```

## Usage

```jsonnet
local drone = import "github.com/Duologic/drone-libsonnet";

drone.pipeline.step.<attribute>

```

## Index

* [`fn dependsOnCloneStep()`](#fn-dependsonclonestep)
* [`fn new(name, image)`](#fn-new)
* [`fn withCommands(value)`](#fn-withcommands)
* [`fn withCommandsMixin(value)`](#fn-withcommandsmixin)
* [`fn withDependsOn(value)`](#fn-withdependson)
* [`fn withDependsOnMixin(value)`](#fn-withdependsonmixin)
* [`fn withDetach(value)`](#fn-withdetach)
* [`fn withEnvironment(value)`](#fn-withenvironment)
* [`fn withEnvironmentMixin(value)`](#fn-withenvironmentmixin)
* [`fn withFailure(value)`](#fn-withfailure)
* [`fn withImage(value)`](#fn-withimage)
* [`fn withName(value)`](#fn-withname)
* [`fn withNetworkMode(value)`](#fn-withnetworkmode)
* [`fn withPrivileged(value)`](#fn-withprivileged)
* [`fn withPull(value)`](#fn-withpull)
* [`fn withSettings(value)`](#fn-withsettings)
* [`fn withSettingsMixin(value)`](#fn-withsettingsmixin)
* [`fn withUser(value)`](#fn-withuser)
* [`fn withVolumes(value)`](#fn-withvolumes)
* [`fn withVolumesMixin(value)`](#fn-withvolumesmixin)
* [`fn withWhen(value)`](#fn-withwhen)
* [`fn withWhenMixin(value)`](#fn-withwhenmixin)
* [`obj volumes`](#obj-volumes)
  * [`fn withName(value)`](#fn-volumeswithname)
  * [`fn withPath(value)`](#fn-volumeswithpath)
* [`obj when`](#obj-when)
  * [`fn onFailure()`](#fn-whenonfailure)
  * [`fn onPullRequest()`](#fn-whenonpullrequest)
  * [`fn onPushToBranch(branch_name)`](#fn-whenonpushtobranch)
  * [`fn onPushToMainBranch()`](#fn-whenonpushtomainbranch)
  * [`fn onPushToMasterBranch()`](#fn-whenonpushtomasterbranch)
  * [`fn onSuccess()`](#fn-whenonsuccess)
  * [`fn withBranch(value)`](#fn-whenwithbranch)
  * [`fn withBranchMixin(value)`](#fn-whenwithbranchmixin)
  * [`fn withCron(value)`](#fn-whenwithcron)
  * [`fn withCronMixin(value)`](#fn-whenwithcronmixin)
  * [`fn withEvent(value)`](#fn-whenwithevent)
  * [`fn withEventMixin(value)`](#fn-whenwitheventmixin)
  * [`fn withPaths(value)`](#fn-whenwithpaths)
  * [`fn withPathsMixin(value)`](#fn-whenwithpathsmixin)
  * [`fn withRef(value)`](#fn-whenwithref)
  * [`fn withRefMixin(value)`](#fn-whenwithrefmixin)
  * [`fn withRepo(value)`](#fn-whenwithrepo)
  * [`fn withRepoMixin(value)`](#fn-whenwithrepomixin)
  * [`fn withStatus(value)`](#fn-whenwithstatus)
  * [`fn withStatusMixin(value)`](#fn-whenwithstatusmixin)
  * [`fn withTarget(value)`](#fn-whenwithtarget)
  * [`fn withTargetMixin(value)`](#fn-whenwithtargetmixin)
  * [`obj branch`](#obj-whenbranch)
    * [`fn withCondition(value)`](#fn-whenbranchwithcondition)
    * [`fn withConditionMixin(value)`](#fn-whenbranchwithconditionmixin)
    * [`fn withExclude(value)`](#fn-whenbranchwithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whenbranchwithexcludemixin)
    * [`fn withInclude(value)`](#fn-whenbranchwithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whenbranchwithincludemixin)
  * [`obj cron`](#obj-whencron)
    * [`fn withCondition(value)`](#fn-whencronwithcondition)
    * [`fn withConditionMixin(value)`](#fn-whencronwithconditionmixin)
    * [`fn withExclude(value)`](#fn-whencronwithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whencronwithexcludemixin)
    * [`fn withInclude(value)`](#fn-whencronwithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whencronwithincludemixin)
  * [`obj event`](#obj-whenevent)
    * [`fn withCondition(value)`](#fn-wheneventwithcondition)
    * [`fn withConditionMixin(value)`](#fn-wheneventwithconditionmixin)
    * [`fn withExclude(value)`](#fn-wheneventwithexclude)
    * [`fn withExcludeMixin(value)`](#fn-wheneventwithexcludemixin)
    * [`fn withInclude(value)`](#fn-wheneventwithinclude)
    * [`fn withIncludeMixin(value)`](#fn-wheneventwithincludemixin)
  * [`obj paths`](#obj-whenpaths)
    * [`fn withCondition(value)`](#fn-whenpathswithcondition)
    * [`fn withConditionMixin(value)`](#fn-whenpathswithconditionmixin)
    * [`fn withExclude(value)`](#fn-whenpathswithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whenpathswithexcludemixin)
    * [`fn withInclude(value)`](#fn-whenpathswithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whenpathswithincludemixin)
  * [`obj ref`](#obj-whenref)
    * [`fn withCondition(value)`](#fn-whenrefwithcondition)
    * [`fn withConditionMixin(value)`](#fn-whenrefwithconditionmixin)
    * [`fn withExclude(value)`](#fn-whenrefwithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whenrefwithexcludemixin)
    * [`fn withInclude(value)`](#fn-whenrefwithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whenrefwithincludemixin)
  * [`obj repo`](#obj-whenrepo)
    * [`fn withCondition(value)`](#fn-whenrepowithcondition)
    * [`fn withConditionMixin(value)`](#fn-whenrepowithconditionmixin)
    * [`fn withExclude(value)`](#fn-whenrepowithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whenrepowithexcludemixin)
    * [`fn withInclude(value)`](#fn-whenrepowithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whenrepowithincludemixin)
  * [`obj target`](#obj-whentarget)
    * [`fn withCondition(value)`](#fn-whentargetwithcondition)
    * [`fn withConditionMixin(value)`](#fn-whentargetwithconditionmixin)
    * [`fn withExclude(value)`](#fn-whentargetwithexclude)
    * [`fn withExcludeMixin(value)`](#fn-whentargetwithexcludemixin)
    * [`fn withInclude(value)`](#fn-whentargetwithinclude)
    * [`fn withIncludeMixin(value)`](#fn-whentargetwithincludemixin)

## Fields

### fn dependsOnCloneStep

```ts
dependsOnCloneStep()
```

`dependsOnCloneStep` is a shorthand for `withDependsOn(['clone'])


### fn new

```ts
new(name, image)
```

`new` is a shorthand for creating a new step object

### fn withCommands

```ts
withCommands(value)
```



### fn withCommandsMixin

```ts
withCommandsMixin(value)
```



### fn withDependsOn

```ts
withDependsOn(value)
```



### fn withDependsOnMixin

```ts
withDependsOnMixin(value)
```



### fn withDetach

```ts
withDetach(value)
```



### fn withEnvironment

```ts
withEnvironment(value)
```



### fn withEnvironmentMixin

```ts
withEnvironmentMixin(value)
```



### fn withFailure

```ts
withFailure(value)
```



### fn withImage

```ts
withImage(value)
```



### fn withName

```ts
withName(value)
```



### fn withNetworkMode

```ts
withNetworkMode(value)
```



### fn withPrivileged

```ts
withPrivileged(value)
```



### fn withPull

```ts
withPull(value)
```



### fn withSettings

```ts
withSettings(value)
```



### fn withSettingsMixin

```ts
withSettingsMixin(value)
```



### fn withUser

```ts
withUser(value)
```



### fn withVolumes

```ts
withVolumes(value)
```



### fn withVolumesMixin

```ts
withVolumesMixin(value)
```



### fn withWhen

```ts
withWhen(value)
```



### fn withWhenMixin

```ts
withWhenMixin(value)
```



### obj volumes


#### fn volumes.withName

```ts
withName(value)
```



#### fn volumes.withPath

```ts
withPath(value)
```



### obj when


#### fn when.onFailure

```ts
onFailure()
```

`onFailure` will conditionally limit this step to a pipeline failure.


#### fn when.onPullRequest

```ts
onPullRequest()
```

`onPullRequest` will conditionally limit this step to
a `pull_request` event


#### fn when.onPushToBranch

```ts
onPushToBranch(branch_name)
```

`onPushToBranch` will conditionally limit this step to a `push` event
on `<branch_name>`


#### fn when.onPushToMainBranch

```ts
onPushToMainBranch()
```

`onPushToMainBranch` will conditionally limit this step to a `push`
event on `main` branch


#### fn when.onPushToMasterBranch

```ts
onPushToMasterBranch()
```

`onPushToMasterBranch` will conditionally limit this step to a `push`
event on `master` branch


#### fn when.onSuccess

```ts
onSuccess()
```

`onSuccess` will conditionally limit this step to a successful
pipeline


#### fn when.withBranch

```ts
withBranch(value)
```



#### fn when.withBranchMixin

```ts
withBranchMixin(value)
```



#### fn when.withCron

```ts
withCron(value)
```



#### fn when.withCronMixin

```ts
withCronMixin(value)
```



#### fn when.withEvent

```ts
withEvent(value)
```



#### fn when.withEventMixin

```ts
withEventMixin(value)
```



#### fn when.withPaths

```ts
withPaths(value)
```



#### fn when.withPathsMixin

```ts
withPathsMixin(value)
```



#### fn when.withRef

```ts
withRef(value)
```



#### fn when.withRefMixin

```ts
withRefMixin(value)
```



#### fn when.withRepo

```ts
withRepo(value)
```



#### fn when.withRepoMixin

```ts
withRepoMixin(value)
```



#### fn when.withStatus

```ts
withStatus(value)
```



#### fn when.withStatusMixin

```ts
withStatusMixin(value)
```



#### fn when.withTarget

```ts
withTarget(value)
```



#### fn when.withTargetMixin

```ts
withTargetMixin(value)
```



#### obj when.branch


##### fn when.branch.withCondition

```ts
withCondition(value)
```



##### fn when.branch.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.branch.withExclude

```ts
withExclude(value)
```



##### fn when.branch.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.branch.withInclude

```ts
withInclude(value)
```



##### fn when.branch.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.cron


##### fn when.cron.withCondition

```ts
withCondition(value)
```



##### fn when.cron.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.cron.withExclude

```ts
withExclude(value)
```



##### fn when.cron.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.cron.withInclude

```ts
withInclude(value)
```



##### fn when.cron.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.event


##### fn when.event.withCondition

```ts
withCondition(value)
```



##### fn when.event.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.event.withExclude

```ts
withExclude(value)
```



##### fn when.event.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.event.withInclude

```ts
withInclude(value)
```



##### fn when.event.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.paths


##### fn when.paths.withCondition

```ts
withCondition(value)
```



##### fn when.paths.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.paths.withExclude

```ts
withExclude(value)
```



##### fn when.paths.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.paths.withInclude

```ts
withInclude(value)
```



##### fn when.paths.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.ref


##### fn when.ref.withCondition

```ts
withCondition(value)
```



##### fn when.ref.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.ref.withExclude

```ts
withExclude(value)
```



##### fn when.ref.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.ref.withInclude

```ts
withInclude(value)
```



##### fn when.ref.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.repo


##### fn when.repo.withCondition

```ts
withCondition(value)
```



##### fn when.repo.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.repo.withExclude

```ts
withExclude(value)
```



##### fn when.repo.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.repo.withInclude

```ts
withInclude(value)
```



##### fn when.repo.withIncludeMixin

```ts
withIncludeMixin(value)
```



#### obj when.target


##### fn when.target.withCondition

```ts
withCondition(value)
```



##### fn when.target.withConditionMixin

```ts
withConditionMixin(value)
```



##### fn when.target.withExclude

```ts
withExclude(value)
```



##### fn when.target.withExcludeMixin

```ts
withExcludeMixin(value)
```



##### fn when.target.withInclude

```ts
withInclude(value)
```



##### fn when.target.withIncludeMixin

```ts
withIncludeMixin(value)
```


