App.N2oBonusItemView = Em.View.extend Em.TargetActionSupport,
  templateName: 'templates/n2o/bonus-item'
  classNames: ['n2o_bonus_item_view']
  classNameBindings: ['completed:completed', 'disabled', 'hidden:hide']
  tagName: 'li'

  name: null
  icon: null
  completed: null
  reward: 0
  route: null
  url: null
  enabled: true
  hidden: false

  popup: false
  width: null
  height: null

  oauth: false # OAuth actions - redirect back to referrer
  service: null # Needs to be set only for OAuth actions

  disabled: (->
    !@get('enabled')
  ).property 'enabled'

  completedOauthItem: (->
    @get('oauth') && @get('completed')
  ).property 'oauth', 'completed'

  didClickDisconnect: (event) ->
    @get('controller').disconnectService @get('service')
    event.stopPropagation()

  click: ->
    disabled = @get 'disabled'
    completed = @get 'completed'

    unless disabled || completed
      if route = @get 'route'
        App.get('router').transitionTo route
      else if url = @get 'url'
        if @get 'oauth'
          url = "#{url}?origin=#{window.encodeURIComponent(window.location.href)}"

        if @get 'popup'
          document.cookie = 'oauth_popup=1; path=/'
          popupOpts = 'resizable=yes,scrollbars=yes,toolbar=yes,status=yes'
          popupOpts = "#{popupOpts},width=#{@get 'width'}" if @get 'width'
          popupOpts = "#{popupOpts},height=#{@get 'height'}" if @get 'height'
          window.open(url, 'popup', popupOpts);
        else
          window.location = url
      else
        this.triggerAction()
