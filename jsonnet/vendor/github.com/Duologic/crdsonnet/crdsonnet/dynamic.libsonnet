local d = import 'github.com/jsonnet-libs/docsonnet/doc-util/main.libsonnet';
{
  local this = self,

  nilvalue:: {},

  nestInParents(name, parents, object)::
    std.foldr(
      function(p, acc)
        if p == name
        then acc
        else { [p]+: acc }
      ,
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

  functionHelp(functionName, schema):: {
    ['#%s' % functionName]::
      d.fn(
        help=(if 'description' in schema
              then schema.description
              else ''),
        args=[d.arg(
          'value',
          (if 'type' in schema
           then schema.type
           else 'string')
        )]
      ),
  },

  withFunction(schema)::
    this.functionHelp(this.functionName(schema._name), schema)
    + (if 'default' in schema
       then {
         [this.functionName(schema._name)](value=schema.default):
           this.nestInParents(schema._name, schema._parents, { [schema._name]: value }),
       }
       else {
         [this.functionName(schema._name)](value):
           this.nestInParents(schema._name, schema._parents, { [schema._name]: value }),
       }),

  withConstant(schema)::
    this.functionHelp(this.functionName(schema._name), schema)
    + {
      [this.functionName(schema._name)]():
        this.nestInParents(schema._name, schema._parents, { [schema._name]: schema.const }),
    },

  withBoolean(schema)::
    this.functionHelp(this.functionName(schema._name), schema)
    + {
      [this.functionName(schema._name)](value=true):
        this.nestInParents(schema._name, schema._parents, { [schema._name]: value }),
    },

  mixinFunction(schema)::
    this.functionHelp(this.functionName(schema._name) + 'Mixin', schema)
    + {
      [this.functionName(schema._name) + 'Mixin'](value):
        this.nestInParents(schema._name, schema._parents, { [schema._name]+: value }),
    },

  arrayFunctions(schema)::
    this.functionHelp(this.functionName(schema._name), schema)
    + this.functionHelp(this.functionName(schema._name) + 'Mixin', schema)
    + {
      [this.functionName(schema._name)](value):
        this.nestInParents(
          schema._name,
          schema._parents,
          this.named(schema._name, if std.isArray(value) then value else [value])
        ),

      [this.functionName(schema._name) + 'Mixin'](value):
        this.nestInParents(
          schema._name,
          schema._parents,
          this.named(schema._name, if std.isArray(value) then value else [value])
        ),
    },

  named(name, object)::
    {
      [name]+: object,
    },

  toObject(object)::
    object,

  newFunction(parents)::
    this.nestInParents(
      'new',
      parents,
      {
        new(name):
          self.withApiVersion()
          + self.withKind()
          + self.metadata.withName(name),
      },
    ),
}

// vim: foldmethod=indent
