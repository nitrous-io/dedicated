App.util =
  UserAgent: new UAParser().getResult()

  IsMac:
    navigator.userAgent.indexOf("Macintosh") != -1

  IsMobile:
    navigator.userAgent.match(/(Android|iPad|iPhone)/) != null

  # TODO determine the present location without it being passed
  saveReturnTo: (route)->
    App.set('returnTo', route)

  returnToSaved: (defaultRoute) ->
    route = if App.get('returnTo')?
      App.get('returnTo')
    else
      defaultRoute
    App.get('router').transitionTo route

  mixpanel: (eventName, properties = {}) ->
    currentUser = App.get 'currentUser'
    if mixpanel? && !currentUser.get('admin')
      p =
        username:         currentUser.get('username')
        email:            currentUser.get('email')
        distinct_id:      currentUser.get('email')
        os_name:          @UserAgent.os.name
        os_version:       @UserAgent.os.version
        browser_name:     @UserAgent.browser.name
        browser_version:  @UserAgent.browser.version
        browser_major:    @UserAgent.browser.major

      mixpanel.track eventName, _.defaults(properties, p)

  mixpanelIdentifyUser: ->
    if mixpanel?
      currentUser = App.get 'currentUser'
      currentUserFullName = currentUser.get('full_name')

      # alias should only be used when the user signed up
      # https://mixpanel.com/docs/integration-libraries/using-mixpanel-alias
      mixpanel.identify currentUser.get('email')

      personHash =
        $email: currentUser.get('email')
        $username: currentUser.get('username')
        $created: currentUser.get('created_at')
        $name: currentUserFullName
        is_github_authenticated: currentUser.get('isGithubAuthenticated')

      if currentUserFullName?
        splitName = currentUserFullName.split(' ')
        personHash.$first_name = splitName[0]
        personHash.$last_name  = splitName.slice(-1)[0]

      mixpanel.people.set personHash

  mixpanelIncrement: (property, value=1) ->
    if mixpanel? && !App.get('currentUser.admin')
      mixpanel.people.increment(property, value)

  mixpanelUpdateUser: (properties) ->
    return if !mixpanel?
    mixpanel.people.set properties

  mixpanelAppendUser: (properties) ->
    return if !mixpanel?
    mixpanel.people.append properties

  # Algo stolen from http://ejohn.org/files/pretty.js
  humanizedTimeProperty: (property) ->
    return (->
      val = @get(property)
      # Assume property is in UTC
      val = if _.isDate(val) then val.toISOString() else val
      date = new Date((val || "").replace(/-/g,"/").replace(/[TZ]/g," "))
      # Get now in UTC as well
      now = new Date(new Date().toISOString().replace(/-/g,"/").replace(/[TZ]/g," "))

      diff = (now.getTime() - date.getTime()) / 1000
      day_diff = Math.floor(diff / 86400)

      if isNaN(day_diff) || day_diff < 0
        return

      day_diff == 0 && (
                diff < 60 && "just now" ||
                diff < 120 && "a minute ago" ||
                diff < 3600 && Math.floor( diff / 60 ) + " minutes ago" ||
                diff < 7200 && "an hour ago" ||
                diff < 86400 && Math.floor( diff / 3600 ) + " hours ago") ||
                day_diff == 1 && "yesterday" ||
                day_diff < 7 && day_diff + " days ago" ||
                day_diff < 31 && Math.ceil( day_diff / 7 ) + " weeks ago" ||
                day_diff < 365 && Math.floor( day_diff / 30 ) + " months ago" ||
                day_diff < 730 && "last year" ||
                Math.floor( day_diff / 365 ) + " years ago"
    ).property(property)

  strftime: (format, date) ->
    strftime(format, date)

  randomElement: (array) ->
    array[Math.floor(Math.random() * array.length)]

  autolink: (text) ->
    urlPattern = ///
      (^|\s)                                # Capture the beginning of string or leading whitespace
      (
        (?:https?|ftp)://                   # Look for a valid URL protocol (non-captured)
        [\-A-Z0-9+\u0026@#/%?=()~_|!:,.;]*  # Valid URL characters (any number of times)
        [\-A-Z0-9+\u0026@#/%=~()_|]         # String must end in a valid URL character
      )
    ///gi

    Handlebars.Utils.escapeExpression(text)
                    .replace( urlPattern, (match, space, url) ->
                      link = "<a href='#{url}' target='_blank'>#{url}</a>"
                      "#{space}#{link}"
                    ).htmlSafe()

  asCurrency: (value) ->
    "$#{value.toFixed(2)}"

  pluralize: (count, singularForm, pluralForm) ->
    term = if count == 1 then singularForm else pluralForm
    [count, term].join(' ')

  connectionError: ->
    App.set 'flashMessage', type: 'error', text: 'Error connecting to server.  Please contact support@nitrous.io'
    this.mixpanel 'connection_error'
