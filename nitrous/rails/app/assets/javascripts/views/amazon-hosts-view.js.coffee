App.AmazonHostsView = Em.View.extend
  elementId: 'amazon-hosts-view'
  classNames: ['admin', 'ui', 'card']

  actions:
    updateHosts: (hosts)->
      selectedHosts = hosts.filterBy('selected', true)
      App.trigger 'show-update-hosts', selectedHosts
