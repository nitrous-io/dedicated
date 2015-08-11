#= require ./invoice

App.OrgInvoice = klass = App.Invoice.extend
  orgSlug: null

klass.reopenClass
  url: '/orgs/:orgSlug/invoices'
