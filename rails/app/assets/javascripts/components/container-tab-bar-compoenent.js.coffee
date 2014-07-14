App.ContainerTabBarComponent = Em.Component.extend
  tagName: 'ul'
  classNames: ['ui', 'tab-bar', 'container-tab-bar']

  selected: null

  generalSelected: Em.computed.equal 'selected', 'general'
  sshSelected:     Em.computed.equal 'selected', 'ssh'
  volumesSelected: Em.computed.equal 'selected', 'volumes'
  portsSelected:   Em.computed.equal 'selected', 'ports'
  envSelected:     Em.computed.equal 'selected', 'env'
  logsSelected:    Em.computed.equal 'selected', 'logs'
  imagesSelected:  Em.computed.equal 'selected', 'images'
  usersSelected:   Em.computed.equal 'selected', 'users'
