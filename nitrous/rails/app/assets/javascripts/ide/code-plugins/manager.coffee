window.App ||= {}

App.CodePluginsManager =
  klasses: {}
  aliases: {}

  globalPlugins: {}

  # track all plugins so we can perform updates
  # on key variables (opSocket etc)
  # maps App.Document objects to plugin instances
  activePlugins: {}
  activeGlobalPlugins: {}

  availableOptions: {}

  setAvailableOption: (optName, value)->
    @availableOptions[optName] = value

    for type, plugin of @activePlugins
      plugin.setOption(optName, value)

    for type, plugin of @activeGlobalPlugins
      plugin.setOption(optName, value)

  registerCodePlugin: (type, klass, aliases = [])->
    klass.pluginType = type
    @klasses[type] = klass
    for alias in aliases
      @aliases[alias] = type

  registerGlobalPlugin: (type, klass)->
    @globalPlugins[type] = klass

  forType: (type)->
    @klasses[type] || @klasses[@aliases[type]]

  initPluginForType: (doc, mode)->
    if plType = @forType(mode)
      if plType.beta
        return null unless @availableOptions.userSettings?.beta

      pl = new plType(doc)
      @setOptions(pl)

      @activePlugins[doc] = pl

      if gblType = @globalPlugins[plType.pluginType]
        unless @activeGlobalPlugins[plType.pluginType]
          gblPlugin = new gblType()
          @setOptions(gblPlugin)

          @activeGlobalPlugins[plType.pluginType] = gblPlugin

      pl.globalPlugin = @activeGlobalPlugins[plType.pluginType]

      pl.afterInitialize()
      return pl

    null

  setOptions: (pluginInstance)->
    for opt, value of @availableOptions
      pluginInstance.setOption(opt, value)