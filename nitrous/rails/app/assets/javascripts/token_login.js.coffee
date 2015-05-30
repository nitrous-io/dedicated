main = ->
  # if query string is not found, redirect to 403 page
  if location.search[0] != '?'
    return redirect '/403'

  # parse query string
  query = {}
  for q in location.search.slice(1).split('&')
    m = q.match(/^([^=]+)=(.*)*$/)
    k = m[1]
    v = m[2]
    if k && v
      query[k] = window.decodeURIComponent(v)

  loginToken = query.token
  redirectUrl = query.redirect_to || '/app'

  obtainAccessToken = ->
    if !loginToken
      return redirect '/403'
    $.ajax(
      type: 'POST'
      url: '/api/v0/oauth/token'
      dataType: 'json'
      data:
        grant_type: 'login_token'
        token: loginToken
    ).done((data) ->
      if data.token_type == 'Bearer' && data.access_token
        # oauth token obtained
        localStorage.setItem('token', data.access_token)
        redirect(redirectUrl)
      else
        redirect '/403'
    ).fail ->
      # failed to obtain new oauth token. redirect to 403
      redirect '/403'

  # first, if oauth token exists in local storage,
  # check if it is still valid
  if existingToken = localStorage.getItem('token')
    $.ajax(
      type: 'GET'
      url: '/api/v0/user'
      dataType: 'json'
      headers:
        Authorization: "Bearer #{existingToken}"
    ).done((data)->
      # existing oauth token is still valid, just redirect
      redirect(redirectUrl)
    ).fail ->
      # existing oauth token is invalid, delete token
      localStorage.removeItem('token')
      obtainAccessToken()
  else
    obtainAccessToken()

redirect = (path='') ->
  if path[0] != '/'
    path = '/' + path
  location.replace(location.protocol + '//' + location.host + path)

main()
