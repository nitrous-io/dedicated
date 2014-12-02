App.BoxResourcesRoute = Em.Route.extend
  afterModel: (box) ->
    unless box.get('ownBox')
      @transitionTo 'boxes.index'

  activate: ->
    App.get('currentUser').fetchN2O()

  deactivate: ->
    @controller.set 'content', null
