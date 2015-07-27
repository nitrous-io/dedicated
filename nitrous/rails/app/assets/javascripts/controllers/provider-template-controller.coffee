parseConfig = (config) ->
  config = JSON.parse(config)

  for k, v of config
    if typeof(v) != 'string'
      throw new Error('Config json may only contain string values')

  return config

App.ProviderTemplateController = Em.ObjectController.extend
  saving: false
  actions:
    save: ->
      pt = @get('model')

      try
        pt.set 'config', parseConfig(@get('configString'))
      catch err
        App.postError(err.message, 'Invalid config json!')
        return

      @set 'saving', true

      pt.save().then(
        () =>
          App.snackbar.post "Provider template updated."
        ,(jqxhr) =>
          @set 'saving', false
          App.postError(jqxhr.responseJSON, 'Failed to save provider template!')
      ).finally(=> 
          @set 'saving', false
      )
