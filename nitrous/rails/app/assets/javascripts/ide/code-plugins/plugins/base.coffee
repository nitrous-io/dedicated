class @PluginBase
  constructor: (@document) ->

  afterSave: (context)->

  goToDefinition: (context)->
    
  path: ->
    @document.get 'path'