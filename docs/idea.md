# Description
Console utility for cleanig stale dependencies from a local maven repository (~/.m2 folder).
Java project folders are scanned recursively and all component in the local maven repository that are not in use are deleted

# Requirements and flow suggestion
The utility has to receive as an argument a path in file system that all maven java projects will be scanned
recursively and build a list of dependencies in use.
All project folders are visited recursively and all dependencies are also calculated recursively
All dependencies that are not in use are subject for removal from the local maven repository
The utility should accept arguments as 
* make deletion list only (no deletion performed)
* perform deletion with a list content
* scan and delete
Use GoLang as programming language
Review the idea and suggest alternative for implementation if applicable