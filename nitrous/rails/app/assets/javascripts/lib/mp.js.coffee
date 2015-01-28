#= require ua-parser

window.mp =
  UserAgent: new UAParser().getResult()

  initialized: false
  identified: false

  init: (token) ->
    return false unless window.mixpanel?
    return false if !token || token == 'disable'

    # initialize mixpanel
    window.mixpanel.init(token)
    @initialized = true
    @identify()

  identify: ->
    return false unless @initialized
    return false unless user = App?.get('user')
    name = user.get('name')

    # alias should only be used when the user signed up
    # https://mixpanel.com/docs/integration-libraries/using-mixpanel-alias
    mixpanel.identify user.get('email')

    createdAt = JSON.parse(JSON.stringify(user.get('createdAt') || user.get('created_at')))

    personHash =
      $email: user.get('email')
      $username: user.get('username')
      $created: createdAt
      $name: name
      user_id: user.get('id').toString()

    if name?
      if splitName = name.match(/(.*)\s([^\s]+)?$/)
        personHash.$first_name = splitName[1]
        personHash.$last_name  = splitName[2]
      else
        personHash.$first_name = name

    mixpanel.people.set personHash
    @identified = true

  track: (eventName, properties = {}) ->
    return false unless @initialized
    return false unless user = App?.get 'user'

    if mixpanel? && !user.get('admin')
      defaultProps =
        username:         user.get('username')
        email:            user.get('email')
        distinct_id:      user.get('id').toString()
        os_name:          @UserAgent.os.name
        os_version:       @UserAgent.os.version
        browser_name:     @UserAgent.browser.name
        browser_version:  @UserAgent.browser.version
        browser_major:    @UserAgent.browser.major

      mixpanel.track eventName, _.defaults(properties, defaultProps)

  increment: (property, value=1) ->
    return false unless @initialized && @identified
    return false if App?.get('user.admin')
    mixpanel.people.increment(property, value)

  updateUser: (properties) ->
    return false unless @initialized && @identified
    mixpanel.people.set properties

  appendUser: (properties) ->
    return false unless @initialized && @identified
    mixpanel.people.append properties
