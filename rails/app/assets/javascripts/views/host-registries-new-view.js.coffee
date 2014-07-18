App.HostRegistriesNewView = Em.View.extend
  elementId: 'host-registries-new-view'

  didInsertElement: ->
    @_super()
    $urlField = @$('#registries-new-url').val(@$('#docker-hub-btn').data('url'))

    @$('.registry-buttons button').on 'click', ->
      $this = $(this)
      $urlField.val($this.data('url'))
      $this.blur()
      false

  willDestroyElement: ->
    @_super()
    @$('.registry-buttons button').off()
