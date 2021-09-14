#!groovy

milestone 0
timestamps {
    node('docker') {
        def commitHash = checkout(scm).GIT_COMMIT

        docker.withRegistry('https://harbor.cyverse.org', 'jenkins-harbor-credentials') {
            def dockerImage
            stage('Build') {
                milestone 50
                dockerImage = docker.build("harbor.cyverse.org/de/image-janitor:${env.BUILD_TAG}", "--build-arg git_commit=${commitHash} .")
                milestone 51
                dockerImage.push()
            }
            stage('Test') {
                dockerTestRunner = "test-${env.BUILD_TAG}"
                try {
                    sh "docker run --rm --entrypoint 'sh' ${dockerImage.imageName()} \"go test -v github.com/cyverse-de/image-janitor | tee /dev/stderr | go-junit-report\" > test-results.xml"
                } finally {
                    junit 'test-results.xml'

                    sh "docker run --rm -v \$(pwd):/build -w /build alpine rm -r test-results.xml"
                }
            }
            stage('Docker Push') {
                milestone 100
                dockerImage.push("${env.BRANCH_NAME}")
                // Retag to 'qa' if this is master/main (keep both so when it switches this keeps working)
                if ( "${env.BRANCH_NAME}" == "master" || "${env.BRANCH_NAME}" == "main" ) {
                    dockerImage.push("qa")
                }
                milestone 101
            }
        }
    }
}
