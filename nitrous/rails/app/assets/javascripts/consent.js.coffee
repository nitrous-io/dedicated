#= require querystring

fetchClient = (clientId, token) ->
  $.ajax 
    type: 'GET'
    url: "/api/v0/oauth/clients/#{clientId}"
    headers: { Authorization: "Bearer #{token}" }
  .then(
    (data) ->
      console.log('fetched client', data)
      data.oauth_client
    , (err) ->
      console.log('error fetching client:', err)
  )

fetchAuthorizationCode = (token, clientId, state, redirectUri) ->
  $.ajax
    type: 'POST'
    url: '/api/v0/oauth/authorize'
    headers: { Authorization: "Bearer #{token}" }
    data:
      response_type: 'code'
      state: state
      client_id: clientId
      redirect_uri: redirectUri
  .then(
    (result) ->
      console.log('got authorization code response', result)
      param = $.param(result)

      # TODO: Merge this with query string in redirect_uri if any
      redirectUri = result.redirect_uri + '?' + param
      console.log('redirecting to', redirectUri)
      window.location = redirectUri

    , (err) ->
      console.log('error getting authorization code:', err)
  )

main = ->
  query = querystring.parse(location.search)
  clientId = query.client_id
  state = query.state
  redirectUri = query.redirect_uri

  unless clientId
    window.location = '/app'

  token = localStorage.getItem('token')
  unless token
    window.location = '/app#/login'

  fetchClient(clientId, token)

  $('.yes-button').click () ->
    console.log('getting authorization token')
    fetchAuthorizationCode(token, clientId, state, redirectUri)

  $('.no-button').click () ->
    console.log('canceling authorization request')

    # TODO: if redirect URL matches fetched client,
    # redirect back to client website.
    window.location = '/app'

main()
