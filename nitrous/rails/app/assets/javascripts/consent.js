//= require querystring
//= require es6-promise

document.addEventListener("DOMContentLoaded", function() {
  'use strict'

  var querystring = window.querystring

  var clientEl = document.getElementById('client-info')
  var clientNameEl = document.getElementById('client-name')
  var usernameEl = document.getElementById('username')
  var yesButton = document.getElementById('yes-button')
  var noButton = document.getElementById('no-button')

  // Valid error codes accepted by OAuth2 specs
  var validErrors = [
    'invalid_request', 'unauthorized_client', 'access_denied',
    'unsupported_response_type', 'invalid_scope', 'server_error',
    'temporarily_unavailable'
  ]

  var ajax = function(params) {
    return Promise.resolve($.ajax(params))
      .catch(function(xhrErr) {
        return Promise.reject(xhrErr.responseJSON)
      })
  }

  var validateRedirectUri = function(originalUri, providedUri) {
    if(!providedUri) return true

    // fragment not allowed
    if(providedUri.indexOf('#') != -1) return false

    return originalUri.split('?')[0] === providedUri.split('?')[0]
  }

  var appendQueryString = function(url, params) {
    var stringified = querystring.stringify(params)

    if(url.indexOf('?') == -1) {
      return url + '?' + stringified
    }

    if(url[url.length-1] != '&') {
      url += '&'
    }

    return url + stringified
  }

  var redirect = function(url, params) {
    if(params) {
      url = appendQueryString(url, params)
    }

    console.log('redirecting to', url)
    // window.location = url
  }

  var redirectError = function(uri, err) {
    var errorCode = err.error || 'server_error'
    var description = err.error_description ||
      'unexpected server error'

    if(validErrors.indexOf(errorCode) == -1) {
      errorCode = 'server_error'
    }

    redirect(uri, {
      error: errorCode,
      error_description: description
    })
  }

  var fetchClient = function(token, clientId) {
    return ajax({
      type: 'GET',
      url: '/api/v0/oauth/clients/' + clientId,
      headers: { 
        Authorization: 'Bearer ' + token  
      }
    }).then(function(data) {
      return data.oauth_client
    })
  }

  var fetchUser = function(token) {
    return ajax({
      type: 'GET',
      url: '/api/v0/user',
      headers: { 
        Authorization: 'Bearer ' + token  
      }
    }).then(function(data) {
      return data.user
    })
  }

  var fetchAuthorizationCode = function(token, clientId, state, redirectUri) {
    return ajax({
      type: 'POST',
      url: '/api/v0/oauth/authorize',
      headers: { 
        Authorization: 'Bearer ' + token 
      },
      data: {
        response_type: 'code',
        state: state,
        client_id: clientId,
        redirect_uri: redirectUri
      }
    })
  }

  var showConsentPage = function(token, query, client, user) {
    var state = query.state
    var clientId = query.client_id
    var clientName = client.name
    var redirectUri = query.redirect_uri
    var username = user.username

    clientNameEl.textContent = clientName
    usernameEl.textContent = username
    clientEl.classList.remove('hide')

    yesButton.addEventListener('click', function() {
      fetchAuthorizationCode(token, clientId, state, redirectUri)
      .then(function(result) {
        redirect(result.redirect_uri, result)
      }, function(err) {
        redirectError(redirectUri, err)
      })
    })

    noButton.addEventListener('click', function() {
      redirectError(redirectUri, {
        error: 'access_denied',
        error_description: 'the resource owner has denied the request'
      })
    })
  }

  var query = querystring.parse(location.search)
  var clientId = query.client_id

  if(!clientId) redirect('/app')

  var token = localStorage.getItem('token')
  if(!token) {
    // TODO: either implement login page here as well
    // or make app#/login redirect back to this page
    return redirect('/app#/login')
  }

  Promise.all([
    fetchClient(token, clientId), 
    fetchUser(token)
  ])
  .then(function(res) {
    var client = res[0]
    var user = res[1]

    if(!validateRedirectUri(client.redirect_uri, query.redirect_uri)) {
      return redirect('/404')
    }

    return showConsentPage(token, query, client, user)
  }, function(err) {
    console.log('failed to load client or user:', err)
    // TODO: display more informative page
    // if client cannot be found or error fetching
    redirect('/404')
  })
  .catch(console.error)

})
