App.HostVolumesView = Em.View.extend
  elementId: 'host-volumes-view'

  actions:
    remove: ->
      volumes = @get 'controller.model.volumes'
      selected = @$('input[name=volume]:checked').map (i, el) ->
        volumes.findBy 'id', parseInt(el.value, 10)
      @get('controller').send 'remove', $.makeArray(selected)
