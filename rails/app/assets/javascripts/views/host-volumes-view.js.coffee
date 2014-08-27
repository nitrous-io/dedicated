App.HostVolumesView = Em.View.extend
  elementId: 'host-volumes-view'

  actions:
    remove: ->
      volumes = @get 'controller.model.volumes'
      selected = @$('input[name=volume]:checked').map (i, el) ->
        volumes.findBy 'slug', el.value
      @get('controller').send 'remove', $.makeArray(selected)
