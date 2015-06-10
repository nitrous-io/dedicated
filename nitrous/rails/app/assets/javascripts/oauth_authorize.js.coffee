#= require_self
#= require webfontloader

window.WebFontConfig =
  google:
    families: [
      'Roboto:300,300italic,400,500:latin'
    ]

  active: ->
    window.setTimeout(->
      window.webFontLoaded?()
    , 1)

main = ->
  loggedIn = false

  # parse query string
  query = {}
  for q in location.search.slice(1).split('&')
    m = q.match(/^([^=]+)=(.*)*$/)
    k = m[1]
    v = m[2]
    if k && v
      query[k] = window.decodeURIComponent(v)

  clientID = query.client_id
  scope = query.scope || 'full'
  state = query.state

  # first, if oauth token exists in local storage,
  # check if it is still valid
  if existingToken = localStorage.getItem('token')
    fetchUser(existingToken).done((data)->
      # existing oauth token is still valid
      loggedIn = true
      username = data.user.username
      $('span.username').text(username)
      $loggedIn = $('#logged-in').show()
      window.setTimeout(->
        $loggedIn.addClass('show')
        window.setTimeout(->
          window.requestAnimationFrame ->
            $loggedIn.addClass('shown')
            enableButtons()
        , 301)
      , 5)
    ).fail ->
      # existing oauth token is invalid, delete token
      localStorage.removeItem('token')
      showLoginForm()
  else
    showLoginForm()

  $dialog = $('.dialog')
  $form = $('#authorize-form')
  $login = $('#login')
  $password = $('#password')

  approve = ->
    accessToken = localStorage.getItem('token')
    $form.find('input[name=access_token]').val(accessToken)
    $form.find('input[name=action]').val('approve')
    $form.submit()

  $('#not-you').click ->
    localStorage.removeItem('token')
    $form.find('input[name=access_token]').val('')
    $loggedIn = $('#logged-in').removeClass('shown').removeClass('show')
    window.setTimeout(->
      $loggedIn.hide()
    , 301)
    showLoginForm()
    loggedIn = false

  $('#authorize-btn').click ->
    $dialog.removeClass('shake')
    $login.attr(disabled: true).removeClass('error')
    $password.attr(disabled: true).removeClass('error')
    if loggedIn
      approve()
    else
      obtainAccessToken($login.val(), $password.val()).done(->
        approve()
      ).fail ->
        $dialog.addClass('shake')
        $login.addClass('error').attr(disabled: false)
        $password.addClass('error').attr(disabled: false)
        $login.select().focus()
    false

  $('#cancel-btn').click ->
    if loggedIn
      accessToken = localStorage.getItem('token')
      $form.find('input[name=access_token]').val(accessToken)
      $form.find('input[name=action]').val('deny')
      $form.submit()
    else
      redirect $form.find('input[name=redirect_uri]').val(),
        error: 'access_denied'
        error_description: 'The resource owner or authorization server denied the request'
        state: state
    false

decodeQuery = (uri) ->
  query = {}
  i = uri.indexOf('?')
  if i == -1
    return query
  for q in uri.slice(i + 1).split('&')
    m = q.match(/^([^=]+)=(.*)*$/)
    k = m[1]
    v = m[2]
    if k && v
      query[k] = window.decodeURIComponent(v)
  query

encodeQuery = (queryHash) ->
  q = []
  for k, v of queryHash
    q.push("#{window.encodeURIComponent(k)}=#{window.encodeURIComponent(v)}")
  q.join('&')

redirect = (uri, queryHash) ->
  q = decodeQuery(uri)
  for k, v of queryHash
    q[k] = v
  i = uri.indexOf('?')
  if i != -1
    uri = uri.slice(0, i)
  ruri = uri + '?' + encodeQuery(q)
  location.replace(ruri)

fetchUser = (token) ->
  $.ajax
    type: 'GET'
    url: '/api/v0/user'
    dataType: 'json'
    headers:
      Authorization: "Bearer #{token}"

obtainAccessToken = (login, password) ->
  $.ajax(
    type: 'POST'
    url: '/api/v0/oauth/token'
    dataType: 'json'
    data:
      grant_type: 'password'
      username: login
      password: password
  ).done (data) ->
    if data.token_type == 'Bearer' && data.access_token
      # oauth token obtained
      localStorage.setItem('token', data.access_token)

showLoginForm = ->
  $loginForm = $('#login-form').show()
  window.setTimeout(->
    $loginForm.addClass('show')
    window.setTimeout(->
      window.requestAnimationFrame ->
        $loginForm.addClass('shown')
        enableButtons()
        $('#login').focus()
    , 301)
  , 5)

enableButtons = ->
  $('#cancel-btn').removeClass('disabled')
  $('#authorize-btn').attr(disabled: false)

main()
