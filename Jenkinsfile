// Jenkinsfile - Producto 2 (FP.050 DevOps y Cloud Computing)
// Cada push a GitHub dispara este pipeline:
//   1) Construye la imagen Docker de la web en Go
//   2) La carga en el clúster de Minikube
//   3) La despliega en Kubernetes (2 réplicas + Service balanceador)
//
// Hay dos jobs en Jenkins que usan este mismo archivo:
//   - despliegue-produccion -> rama main    -> namespace "produccion"
//   - despliegue-pruebas    -> rama develop -> namespace "pruebas"

pipeline {
    agent any

    environment {
        APP = 'web-uoc'
        // Etiqueta única por job y por build (ej: despliegue-produccion-7)
        TAG = "${env.JOB_BASE_NAME}-${env.BUILD_NUMBER}"
    }

    stages {
        stage('Build') {
            steps {
                sh 'docker build -t $APP:$TAG .'
            }
        }

        stage('Cargar imagen en Minikube') {
            steps {
                sh 'minikube image load $APP:$TAG'
            }
        }

        stage('Deploy') {
            steps {
                sh '''
                    case "$JOB_BASE_NAME" in
                        *produccion*) NS=produccion ;;
                        *)            NS=pruebas ;;
                    esac
                    echo "Desplegando $APP:$TAG en el namespace $NS"

                    kubectl create namespace $NS --dry-run=client -o yaml | kubectl apply -f -
                    sed "s|IMAGEN|$APP:$TAG|" k8s/deployment.yaml | kubectl apply -n $NS -f -
                    kubectl apply -n $NS -f k8s/service.yaml
                    kubectl rollout status -n $NS deployment/$APP --timeout=180s
                    kubectl get pods,svc -n $NS
                '''
            }
        }
    }
}
