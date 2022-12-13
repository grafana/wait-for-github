# package service



## Install

```
jb install github.com/Duologic/drone-libsonnet@master
```

## Usage

```jsonnet
local drone = import "github.com/Duologic/drone-libsonnet";

drone.pipeline.service.<attribute>

```

## Index

* [`fn withCommand(value)`](#fn-withcommand)
* [`fn withCommandMixin(value)`](#fn-withcommandmixin)
* [`fn withEntrypoint(value)`](#fn-withentrypoint)
* [`fn withEntrypointMixin(value)`](#fn-withentrypointmixin)
* [`fn withEnvironment(value)`](#fn-withenvironment)
* [`fn withEnvironmentMixin(value)`](#fn-withenvironmentmixin)
* [`fn withImage(value)`](#fn-withimage)
* [`fn withName(value)`](#fn-withname)
* [`fn withPrivileged(value)`](#fn-withprivileged)
* [`fn withPull(value)`](#fn-withpull)
* [`fn withVolumes(value)`](#fn-withvolumes)
* [`fn withVolumesMixin(value)`](#fn-withvolumesmixin)
* [`fn withWorkingDir(value)`](#fn-withworkingdir)
* [`obj volumes`](#obj-volumes)
  * [`fn withClaim(value)`](#fn-volumeswithclaim)
  * [`fn withClaimMixin(value)`](#fn-volumeswithclaimmixin)
  * [`fn withConfigMap(value)`](#fn-volumeswithconfigmap)
  * [`fn withConfigMapMixin(value)`](#fn-volumeswithconfigmapmixin)
  * [`fn withHost(value)`](#fn-volumeswithhost)
  * [`fn withHostMixin(value)`](#fn-volumeswithhostmixin)
  * [`fn withName(value)`](#fn-volumeswithname)
  * [`fn withTemp(value)`](#fn-volumeswithtemp)
  * [`fn withTempMixin(value)`](#fn-volumeswithtempmixin)
  * [`obj claim`](#obj-volumesclaim)
    * [`fn withName(value)`](#fn-volumesclaimwithname)
    * [`fn withReadOnly(value)`](#fn-volumesclaimwithreadonly)
  * [`obj config_map`](#obj-volumesconfig_map)
    * [`fn withDefaultMode(value)`](#fn-volumesconfig_mapwithdefaultmode)
    * [`fn withName(value)`](#fn-volumesconfig_mapwithname)
    * [`fn withOptional(value)`](#fn-volumesconfig_mapwithoptional)
  * [`obj host`](#obj-volumeshost)
    * [`fn withPath(value)`](#fn-volumeshostwithpath)
  * [`obj temp`](#obj-volumestemp)
    * [`fn withMedium(value)`](#fn-volumestempwithmedium)

## Fields

### fn withCommand

```ts
withCommand(value)
```



### fn withCommandMixin

```ts
withCommandMixin(value)
```



### fn withEntrypoint

```ts
withEntrypoint(value)
```



### fn withEntrypointMixin

```ts
withEntrypointMixin(value)
```



### fn withEnvironment

```ts
withEnvironment(value)
```



### fn withEnvironmentMixin

```ts
withEnvironmentMixin(value)
```



### fn withImage

```ts
withImage(value)
```



### fn withName

```ts
withName(value)
```



### fn withPrivileged

```ts
withPrivileged(value)
```



### fn withPull

```ts
withPull(value)
```



### fn withVolumes

```ts
withVolumes(value)
```



### fn withVolumesMixin

```ts
withVolumesMixin(value)
```



### fn withWorkingDir

```ts
withWorkingDir(value)
```



### obj volumes


#### fn volumes.withClaim

```ts
withClaim(value)
```



#### fn volumes.withClaimMixin

```ts
withClaimMixin(value)
```



#### fn volumes.withConfigMap

```ts
withConfigMap(value)
```



#### fn volumes.withConfigMapMixin

```ts
withConfigMapMixin(value)
```



#### fn volumes.withHost

```ts
withHost(value)
```



#### fn volumes.withHostMixin

```ts
withHostMixin(value)
```



#### fn volumes.withName

```ts
withName(value)
```



#### fn volumes.withTemp

```ts
withTemp(value)
```



#### fn volumes.withTempMixin

```ts
withTempMixin(value)
```



#### obj volumes.claim


##### fn volumes.claim.withName

```ts
withName(value)
```



##### fn volumes.claim.withReadOnly

```ts
withReadOnly(value)
```



#### obj volumes.config_map


##### fn volumes.config_map.withDefaultMode

```ts
withDefaultMode(value)
```



##### fn volumes.config_map.withName

```ts
withName(value)
```



##### fn volumes.config_map.withOptional

```ts
withOptional(value)
```



#### obj volumes.host


##### fn volumes.host.withPath

```ts
withPath(value)
```



#### obj volumes.temp


##### fn volumes.temp.withMedium

```ts
withMedium(value)
```


