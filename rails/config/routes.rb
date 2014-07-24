Rails.application.routes.draw do
  mount GoAppProxy.new => '/api/v0'
  root 'web_app#index'
end
