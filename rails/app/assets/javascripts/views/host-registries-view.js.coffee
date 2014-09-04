App.HostRegistriesView = Em.View.extend
  elementId: 'host-registries-view'

  actions:
    remove: ->
      registries = @get 'controller.model.registries'
      selected = @$('input[name=registry]:checked').map (i, el) ->
        registries.findBy 'id', parseInt(el.value, 10)
      @get('controller').send 'remove', $.makeArray(selected)
