#= require ../boxes/boxes-control-view

App.IntroN2oView = App.BoxesControlView.extend
  templateName: 'templates/intro/n2o'
  elementId: 'intro_n2o_view'
  classNames: ['app_page', 'intro']

  memoryBinding: 'controller.content.memory'
  storageBinding: 'controller.content.storage'

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Intro Earn N2O Page'

  didClickNextScreen: ->
    box = @get 'controller.content'
    App.get('router').transitionTo 'intro.apps'
