window.App ||= {}

App.CodePluginsManager =
  klasses: {}
  aliases: {}

  registerCodePlugin: (type, klass, aliases = [])->
    @klasses[type] = klass
    for alias in aliases
      @aliases[alias] = type

  forType: (type)->
    @klasses[type] || @klasses[@aliases[type]]
