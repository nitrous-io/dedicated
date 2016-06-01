class Projects < Thor
  desc "find PROJECT_NAME", "Search project with project name"
  def find(name)
    results = Project.query.
      where('projects.name = ?', name)
    Project.from_rows(results).each do |p|
      puts p
    end
  end

  desc "by_user USER_EMAIL", "Search projects with user email"
  def by_user(email)
    results = Project.query.
      where { Sequel.|({users__email: email}) }
    Project.from_rows(results).each do |p|
      puts p
      puts "-" * 30
    end
  end

  desc "recent PROJECT_NUMBER", "List recent deployed new projects"
  def recent(number=10)
    results = Project.query.
      where('active_deployment_id IS NOT NULL').
      where('deployments.deployed_at IS NOT NULL').
      where('deployments.version = 1').
      order(Sequel.desc(:deployments__deployed_at)).
      limit(number)
    Project.from_rows(results).each do |p|
      puts p
      puts "-" * 30
    end
  end
end

# vim:ft=ruby
