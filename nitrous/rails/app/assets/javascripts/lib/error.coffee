Error.create = (attrs) ->
  e = new Error(attrs.message)
  e.type = attrs.type
  e
