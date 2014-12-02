App.IntroCreateBoxView = Em.View.extend
  templateName: 'templates/intro/create-box'
  elementId: 'intro_create_box_view'
  classNames: ['app_page', 'intro', 'new']

  creatingFirstBox: false

  didInsertElement: ->
    @_super()
    App.util.mixpanel 'Viewed Intro Create Box Page'

  markIntroSeen: ->
    App.set 'currentUser.intro_seen', true
    $.ajax(type: 'PUT', url: '/user/intro_seen')

  didClickNextScreen: ->
    this.markIntroSeen()
    @get('controller').save()

  didClickSkipIntro: ->
    this.markIntroSeen()
    @get('controller').cancel()

  willInsertElement: ->
    @_super()
    unless App.get('currentUser.hasUsedN2O')
      @set 'creatingFirstBox', true
