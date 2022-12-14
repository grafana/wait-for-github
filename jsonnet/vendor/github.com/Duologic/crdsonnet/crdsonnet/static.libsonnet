{
  local this = self,

  nilvalue:: '',

  nestInParents(name, parents, object)::
    std.foldr(
      function(p, acc)
        if p == name
        then acc
        else p + '+: { ' + acc + ' }',
      parents,
      object
    ),

  functionName(name)::
    local underscores = std.set(std.findSubstr('_', name));
    local n = std.join('', [
      if std.setMember(i - 1, underscores)
      then std.asciiUpper(name[i])
      else name[i]
      for i in std.range(0, std.length(name) - 1)
      if !std.setMember(i, underscores)
    ]);
    'with' + std.asciiUpper(n[0]) + n[1:],

  withFunction(schema)::
    |||
      %s(value%s): { %s },
    ||| % [
      this.functionName(schema._name),
      (if 'default' in schema
       then '=%s' % (if std.isString(schema.default)
                     then '"%s"' % schema.default
                     else schema.default)
       else ''),
      this.nestInParents(schema._name, schema._parents, schema._name + ': value'),
    ],

  withConstant(schema)::
    |||
      %s(): { %s },
    ||| % [
      this.functionName(schema._name),
      this.nestInParents(schema._name, schema._parents, schema._name + ": '" + schema.const + "'"),
    ],

  withBoolean(schema)::
    |||
      %s(value=%s): { %s },
    ||| % [
      this.functionName(schema._name),
      (if 'default' in schema
       then schema.default
       else 'true'),
      this.nestInParents(schema._name, schema._parents, schema._name + ': value'),
    ],

  mixinFunction(schema)::
    |||
      %sMixin(value): { %s },
    ||| % [
      this.functionName(schema._name),
      this.nestInParents(schema._name, schema._parents, schema._name + '+: value'),
    ],

  arrayFunctions(schema)::
    |||
      %s(value): { %s },
      %sMixin(value): { %s },
    ||| % [
      this.functionName(schema._name),
      this.nestInParents(
        schema._name,
        schema._parents,
        ' %s: if std.isArray(value) then value else [value] ' % schema._name,
      ),
      this.functionName(schema._name),
      this.nestInParents(
        schema._name,
        schema._parents,
        ' %s+: if std.isArray(value) then value else [value] ' % schema._name,
      ),
    ],

  named(name, object)::
    |||
      %s+: %s,
    ||| % [
      name,
      object,
    ],

  toObject(object)::
    '{ %s }' % object,

  newFunction(parents)::
    '{\n %s \n}' %
    this.nestInParents(
      'new',
      parents,
      |||
        new(name):
          self.withApiVersion()
          + self.withKind()
          + self.metadata.withName(name),
      |||,
    ),
}

// vim: foldmethod=indent
