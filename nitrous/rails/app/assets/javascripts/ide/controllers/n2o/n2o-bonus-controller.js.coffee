App.N2oBonusController = Em.ObjectController.extend
  content: null
  bonusInfo: null
  composeTweetDialog: null
  isIntro: false
  hideBusinessSurveyBonus: true

  toggleBusinessSurvey: (->
    if @get('business_survey') || App.get('currentUser.hasCompany')
      @set 'hideBusinessSurveyBonus', false
  ).observes 'business_survey', 'App.currentUser.hasCompany'

  reload: ->
    $.ajax(
      type: 'GET'
      dataType: 'json'
      url: '/bonus'
    ).done (json) =>
      if json && json.bonus
        @get('content')?.loadHash json.bonus
        if !@get('business_survey') && user = App.get('currentUser')
          user.fetchCompany() unless user.get('hasCompany')

  isChromeAppInstalled: ->
    window.chrome?.app?.isInstalled

  notifyChromeAppInstallation: ->
    $.ajax(
      type: 'PUT'
      url: '/user/chrome_app_installed'
    ).always =>
      this.reload()

  followOnTwitter: ->
    $.ajax(type: 'PUT', url: '/bonus/twitter_follow')
    @get('content').set 'twitter_follow', Date.now()

  composeTweet: ->
    @set 'composeTweetDialog', App.ComposeTweetDialog.create()

  sendTweet: (message) ->
    $.ajax(type: 'PUT', url: '/bonus/send_tweet', data: { message: message })
    @get('content').set 'tweet_sent', Date.now()

  disconnectService: (service) ->
    $.ajax(
      url: "/bonus/disconnect/#{service}"
      type: 'DELETE'
      dataType: 'json'
    ).done( =>
      @get('content').set service, null
    ).fail (e) ->
      response = $.parseJSON e.responseText
      error = response.errors?.base?[0]
      App.set 'flashMessage', { type: 'alert-error', text: error }

  _autoRedeemChromeAppBonus: (->
    chromeAppBonusRedeemed = @get('content.chrome_app') != null
    if !chromeAppBonusRedeemed && this.isChromeAppInstalled()
      this.notifyChromeAppInstallation()
  ).observes 'content.chrome_app'
