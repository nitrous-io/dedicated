App.BoxesIndexView = Em.View.extend
  templateName: 'templates/boxes/index'
  classNames: ['app_page', 'index']
  elementId: 'boxes_index_view'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Boxes Index'
    window.setTimeout(->
      $('.confirm-email-reminder')?.slideDown()
    , 500)

  didClickNewBox: ->
    App.get('router').transitionTo 'boxes.new'

  didClickRefresh: ->
    @get('controller').refresh()
