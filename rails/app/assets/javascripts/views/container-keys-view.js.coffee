App.ContainerKeysView = Em.View.extend
  elementId: 'container-keys-view'

  actions:
    remove: ->
      keys = @get 'controller.model.keys'
      selected = @$('input[name=key]:checked').map (i, el) ->
        keys.findBy 'id', parseInt(el.value, 10)
      @get('controller').send 'remove', $.makeArray(selected)
