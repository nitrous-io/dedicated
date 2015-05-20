//= require querystring

var fetchClient = function(clientId, token) {
  return $.ajax({
    type: 'GET',
    url: '/api/v0/oauth/clients/' + clientId,
    headers: { Authorization: 'Bearer ' + token  },
  }).then(
    function(data) {
      console.log('fetched client', data)
      return data.oauth_client
    }, function(err) {
      console.log('error fetching client:', err)
    }
  )
}

var fetchAuthorizationCode = function(token, clientId, state, redirectUri) {
  return $.ajax({
    type: 'POST',
    url: '/api/v0/oauth/authorize',
    headers: { Authorization: 'Bearer ' + token },
    data: {
      response_type: 'code',
      state: state,
      client_id: clientId,
      redirect_uri: redirectUri,
    }
  }).then(
    function(result) {
      console.log('got authorization code response', result)
      var param = $.param(result)

      // TODO: Merge this with query string in redirect_uri if any
      var redirectUri = result.redirect_uri + '?' + param
      console.log('redirecting to', redirectUri)
      window.location = redirectUri

    }, function(err) {
      console.log('error getting authorization code:', err)
    }
  )
}

;(function() {
  var query = querystring.parse(location.search)
  var clientId = query.client_id
  var state = query.state
  var redirectUri = query.redirect_uri

  if(!clientId) {
    window.location = '/app'
  }

  var token = localStorage.getItem('token')
  if(!token) {
    window.location = '/app#/login'
  }

  fetchClient(clientId, token)

  $('.yes-button').click(function() {
    console.log('getting authorization token')
    fetchAuthorizationCode(token, clientId, state, redirectUri)
  })

  $('.no-button').click(function() {
    console.log('canceling authorization request')

    // TODO: if redirect URL matches fetched client,
    // redirect back to client website.
    window.location = '/app'
  })

})()
