package skills

// ResolvePath finds the shortest sequence of nodes connecting start to end via [[wikilinks]].
// Returns nil if no path exists.
func ResolvePath(skill *Skill, startNode, endNode string) []string {
	if skill == nil || skill.Nodes == nil {
		return nil
	}
	if _, ok := skill.Nodes[startNode]; !ok {
		return nil
	}
	if _, ok := skill.Nodes[endNode]; !ok {
		return nil
	}
	if startNode == endNode {
		return []string{startNode}
	}

	type step struct {
		node string
		path []string
	}

	queue := []step{{node: startNode, path: []string{startNode}}}
	visited := map[string]bool{startNode: true}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		node := skill.Nodes[curr.node]
		for _, link := range node.Links {
			if link == endNode {
				return append(curr.path, link)
			}
			if !visited[link] {
				if _, exists := skill.Nodes[link]; exists {
					visited[link] = true
					newPath := make([]string, len(curr.path)+1)
					copy(newPath, curr.path)
					newPath[len(curr.path)] = link
					queue = append(queue, step{node: link, path: newPath})
				}
			}
		}
	}

	return nil
}

// FindBridges looks for intermediate nodes that connect a set of "active" nodes.
// If A and C are in activeNodes, and A -> B -> C exists, B is added as a bridge.
func FindBridges(skill *Skill, activeNodes []string) []string {
	if len(activeNodes) < 2 {
		return nil
	}

	nodeSet := make(map[string]bool)
	for _, n := range activeNodes {
		nodeSet[n] = true
	}

	bridges := make(map[string]bool)
	for i := 0; i < len(activeNodes); i++ {
		for j := i + 1; j < len(activeNodes); j++ {
			path := ResolvePath(skill, activeNodes[i], activeNodes[j])
			if len(path) > 2 {
				// Intermediate nodes are bridges
				for _, p := range path[1 : len(path)-1] {
					if !nodeSet[p] {
						bridges[p] = true
					}
				}
			}
			// Also check reverse path if it's not a DAG
			path = ResolvePath(skill, activeNodes[j], activeNodes[i])
			if len(path) > 2 {
				for _, p := range path[1 : len(path)-1] {
					if !nodeSet[p] {
						bridges[p] = true
					}
				}
			}
		}
	}

	var result []string
	for b := range bridges {
		result = append(result, b)
	}
	return result
}
