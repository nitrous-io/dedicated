App.OrgsListComponent = Em.Component.extend
  layoutName: 'components/orgs/list'
  elementId: 'orgs-list'
  classNames: ['orgs-list-component', 'block-wrapper', 'el-orgs']

  collection: null
  selectedItem: null
