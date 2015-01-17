window.webapp ?= {}

webapp.loading =
  start: ->
    el = document.getElementById('webapp-loading')
    el.className = 'loading'

  stop: ->
    el = document.getElementById('webapp-loading')
    el.className = 'loaded'
    window.setTimeout(->
      el.parentElement.removeChild(el)
    , 500)

window.setTimeout(->
  webapp.loading.start()
 , 1)
