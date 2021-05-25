@Library('dst-shared@master') _

dockerBuildPipeline {
    githubPushRepo = "Cray-HPE/cray-powerdns-manager"
    repository = "cray"
    imagePrefix = ""
    app = "powerdns-manager"
    name = "powerdns-manager"
    description = "Docker image for the PowerDNS manager job"
    dockerfile = "Dockerfile"
    slackNotification = ["", "", false, false, true, true]
    product = "csm"
}
