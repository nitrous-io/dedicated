App.HomeView = Em.View.extend
  elementId: 'home-view'

  noHost: (->
    App.get('hosts.loaded') && App.get('hosts.length') == 0
  ).property 'App.hosts.loaded', 'App.hosts.length'
