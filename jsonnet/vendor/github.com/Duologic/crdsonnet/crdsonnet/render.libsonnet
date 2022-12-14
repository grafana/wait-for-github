{
  static: self.new(import 'static.libsonnet'),
  dynamic: self.new(import 'dynamic.libsonnet'),

  new(r): {
    nilvalue: r.nilvalue,
    toObject: r.toObject,
    nestInParents(parents, object): r.nestInParents('', parents, object),
    newFunction: r.newFunction,

    render(schema):
      r.toObject(self.schema(schema)),

    schema(schema):
      // foldStart
      if 'const' in schema
      then self.const(schema)  // value is a constant

      else if 'type' in schema
      then
        if std.isBoolean(schema.type)
        then
          if schema.type
          then self.other(schema)  // Any value allowed
          else r.nilvalue  // No value allowed

        else if std.isArray(schema.type)
        then self.other(schema)  // Multiple types

        else if schema.type == 'object'
        then self.object(schema)  // type=object

        else if schema.type == 'array'
        then self.array(schema)  // type=array

        else if schema.type == 'boolean'
        then self.boolean(schema)  // type=boolean

        else self.other(schema)  // any other type

      else if 'allOf' in schema
              || 'anyOf' in schema
              || 'oneOf' in schema
      then self.xof(schema)  // value can be xOf

      else self.other(schema)
    ,
    // foldEnd

    nameParsed(schema, parsed):
      // foldStart
      if '_name' in schema
         && parsed != r.nilvalue
      then
        r.named(
          schema._name,
          r.toObject(
            parsed
          )
        )
      else
        parsed
    ,
    // foldEnd

    functions(schema):
      // foldStart
      if std.length(schema._parents) != 0 && '_name' in schema
      then r.withFunction(schema)
           + r.mixinFunction(schema)
      else r.nilvalue,
    // foldEnd

    xofParts(schema):
      // foldStart
      local handle(schema, k) =
        if k in schema
        then
          std.foldl(
            function(acc, n)
              acc + self.schema(n),
            schema[k],
            r.nilvalue
          )
        else r.nilvalue;
      handle(schema, 'allOf')
      + handle(schema, 'anyOf')
      + handle(schema, 'oneOf'),
    // foldEnd

    const(schema): r.withConstant(schema),

    boolean(schema): r.withBoolean(schema),

    other(schema):
      // foldStart
      if std.length(schema._parents) != 0 && '_name' in schema
      then r.withFunction(schema)
      else r.nilvalue,
    //foldEnd

    object(schema):
      // foldStart
      local properties = (
        if 'properties' in schema
        then
          std.foldl(
            function(acc, p)
              acc + self.schema(schema.properties[p]),
            std.objectFields(schema.properties),
            r.nilvalue
          )
        else r.nilvalue
      );

      local xofParts = self.xofParts(schema { _parents: super._parents[1:] });

      local parsed = properties + xofParts;

      self.functions(schema)
      + self.nameParsed(schema, parsed),
    // foldEnd

    array(schema):
      // foldStart
      (if '_name' in schema
       then r.arrayFunctions(schema)
       else r.nilvalue)
      + (
        if 'items' in schema
           && std.isObject(schema.items)
        then self.schema(schema.items { _parents: [] })
        else r.nilvalue
      ),
    // foldEnd

    xof(schema):
      // foldStart
      local parsed = self.xofParts(schema);
      self.functions(schema)
      + self.nameParsed(schema, parsed),
    // foldEnd
  },
}

// vim: foldmethod=marker foldmarker=foldStart,foldEnd foldlevel=0
