#= require ./boxes-control-view

App.BoxesNewView = App.BoxesControlView.extend
  templateName: 'templates/boxes/new'
  elementId: 'boxes_new_view'
  classNames: ['app_page', 'new']

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed New Box'
