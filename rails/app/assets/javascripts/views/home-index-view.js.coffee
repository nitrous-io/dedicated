App.HomeIndexView = Em.View.extend
  elementId: 'home-index-view'

  noHost: (->
    App.get('hosts.loaded') && App.get('hosts.length') == 0
  ).property 'App.hosts.loaded', 'App.hosts.length'
