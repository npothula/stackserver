# stackserver

Please refer classDiagram.png (for objects and their inteaction) and processFlow.png (for various functions interactions) in objModelling folder

Channels (rather than using Conditional Variable associated with Mutex for synchornization primitives) are used wherever it is possible for better performance.

Dockerfile has written with multi-stage build for tinier build image.
Docker commands to build the docker image and run the container.
    docker build --rm -f "Dockerfile" -t stackserver:latest .
    docker run -d -p 8080:8080 stackserver:latest
Haven't tested in docker container

Test Results(passedTests, failedTests) are saved in tests folder.
