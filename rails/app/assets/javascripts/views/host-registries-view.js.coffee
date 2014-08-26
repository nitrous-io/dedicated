App.HostRegistriesView = Em.View.extend
  elementId: 'host-registries-view'

  actions:
    remove: ->
      registries = @get 'controller.model.registries'
      selected = @$('input[name=registry]:checked').map (i, el) ->
        registries.findBy 'url', el.value
      @get('controller').send 'remove', $.makeArray(selected)
