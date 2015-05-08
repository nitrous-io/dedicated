#= require ua-parser
#= require_self
#= require ./mp-events

window.Mp =
  UserAgent: new UAParser().getResult()

  initialized: false
  identified: false

  init: (token) ->
    return false unless window.mixpanel?
    return false if !token || token == 'disable'

    # initialize mixpanel
    mixpanel.init?(token)
    @initialized = true

    @identify()

  identify: ->
    return false unless @initialized
    return true if @identified

    user = App?.get('user')

    # do not track if user is switched from admin
    adminToken = App?.get('adminToken')

    if !user || adminToken
      @identified = false
      return false

    name = user.get('name')

    # alias should only be used when the user signed up
    # https://mixpanel.com/docs/integration-libraries/using-mixpanel-alias
    mixpanel.identify?(user.get('id').toString())

    createdAt = JSON.parse(JSON.stringify(user.get('createdAt') || user.get('created_at')))

    personHash =
      $email: user.get('email')
      $username: user.get('username')
      $created: createdAt
      user_id: user.get('id').toString()

    mixpanel.people?.set_once?(personHash) # only $email may change, handled in backend

    personHash =
      $name: name

    if name?
      if splitName = name.match(/(.*)\s([^\s]+)?$/)
        personHash.$first_name = splitName[1]
        personHash.$last_name  = splitName[2]
      else
        personHash.$first_name = name

    mixpanel.people?.set?(personHash)
    @identified = true

  track: (eventName, properties = {}) ->
    return false unless @initialized && eventName?

    p =
      os_name:          @UserAgent.os.name
      os_version:       @UserAgent.os.version
      browser_name:     @UserAgent.browser.name
      browser_version:  @UserAgent.browser.version
      browser_major:    @UserAgent.browser.major

    if user = App?.get 'user'
      p['username'] = user.get('username')
      p['email'] = user.get('email')
      p['distinct_id'] = user.get('id').toString()

    Object.assign(p, properties)
    mixpanel.register_once?('First Time': 'TRUE')
    mixpanel.track?(eventName, p)
    mixpanel.register?('First Time': 'FALSE')

  increment: (property, value=1) ->
    return false unless @initialized && @identified
    mixpanel.people?.increment?(property, value)

  updateUser: (properties) ->
    return false unless @initialized && @identified
    mixpanel.people?.set?(properties)

  appendUser: (properties) ->
    return false unless @initialized && @identified
    mixpanel.people?.append?(properties)

  unionUser: (properties) ->
    return false unless @initialized && @identified
    mixpanel.people?.union?(properties)

  timestamp: (property, value) ->
    return false unless @initialized && @identified
    if !value?
      value = new Date()
    if value.toISOString
      value = value.toISOString()
    p = {}
    p[property] = value
    mixpanel.people?.set?(p)
