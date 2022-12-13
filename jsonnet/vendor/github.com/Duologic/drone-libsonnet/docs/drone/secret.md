# package secret



## Install

```
jb install github.com/Duologic/drone-libsonnet@master
```

## Usage

```jsonnet
local drone = import "github.com/Duologic/drone-libsonnet";

drone.secret.<attribute>

```

## Index

* [`fn new(name, path, key)`](#fn-new)
* [`fn withData(value)`](#fn-withdata)
* [`fn withGet(value)`](#fn-withget)
* [`fn withGetMixin(value)`](#fn-withgetmixin)
* [`fn withKind(value)`](#fn-withkind)
* [`fn withName(value)`](#fn-withname)
* [`obj get`](#obj-get)
  * [`fn withName(value)`](#fn-getwithname)
  * [`fn withPath(value)`](#fn-getwithpath)

## Fields

### fn new

```ts
new(name, path, key)
```

`new` is a shorthand for creating a new secret object

### fn withData

```ts
withData(value)
```



### fn withGet

```ts
withGet(value)
```



### fn withGetMixin

```ts
withGetMixin(value)
```



### fn withKind

```ts
withKind(value)
```



### fn withName

```ts
withName(value)
```



### obj get


#### fn get.withName

```ts
withName(value)
```



#### fn get.withPath

```ts
withPath(value)
```


