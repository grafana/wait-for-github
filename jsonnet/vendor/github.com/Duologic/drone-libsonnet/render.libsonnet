{
  local this = self,

  local kinds = [
    'signature',
    'secret',
    'template',
    'pipeline',
  ],

  getDroneObjects(object)::
    if std.isObject(object)
    then
      // a Drone object is characterized by having a Kind with values listed above
      if std.objectHas(object, 'kind') && std.member(kinds, object.kind)
      then [object]
      else
        std.foldl(
          function(acc, o)
            acc + this.getDroneObjects(object[o]),
          std.objectFields(object),
          []
        )
    else if std.isArray(object)
    then
      std.flattenArrays(
        std.map(
          function(obj)
            this.getDroneObjects(obj),
          object
        )
      )
    else [],

  toYaml(objects):
    std.manifestYamlStream(
      self.getDroneObjects(objects),
    ),
}
