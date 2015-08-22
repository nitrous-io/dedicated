class @PluginBase
  constructor: (@document) ->

  afterSave: (context)->
  
  supportsGoToDefinition: false
  goToDefinition: (context)->

  path: ->
    @document.get 'path'

  handleError: (err)->
    # this is an example for the plugin proposal, we would not use alert in RL
    window.alert("#{@document.get('title')}: #{err}")